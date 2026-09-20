package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// How full the cluster is (2026-09-20).
//
// # Why this exists beside the usage route
//
// Usage needs a metrics-server, and Talos ships none, so on most clusters the
// usage screen correctly says nobody is measuring. That answer is honest and it
// is not what somebody asking "how full is this" needs.
//
// This is the figure that exists on EVERY cluster: allocatable against what the
// pods asked for. It is what the scheduler works with, so it is also the number
// that decides whether the next pod starts -- which is the question behind
// "how full is it" more often than actual consumption is.
//
// Both are kept and neither is called the other. A cluster at 90% requested and
// 5% used is over-reserved and will refuse work it could do; one at 20%
// requested and 95% used is about to fall over while looking empty. Those are
// opposite repairs, and a screen that showed one number would send somebody the
// wrong way half the time.

// Capacity is how much of something there is and how much is spoken for.
type Capacity struct {
	// Allocatable is what the kubelet offers the scheduler: the machine minus
	// what Talos and the system reserve. It is not the machine's size.
	Allocatable string `json:"allocatable"`
	Requested   string `json:"requested"`

	// Percent is requested of allocatable, rounded. -1 when allocatable is
	// unknown -- a node that has not reported yet is not a node at 0%.
	Percent int `json:"percent"`
}

// NodeCapacity is one node's room, with what it is.
type NodeCapacity struct {
	Name        string   `json:"name"`
	Ready       bool     `json:"ready"`
	Cordoned    bool     `json:"cordoned"`
	CPU         Capacity `json:"cpu"`
	Memory      Capacity `json:"memory"`
	Pods        Capacity `json:"pods"`
	PodsRunning int      `json:"pods_running"`
}

// ClusterCapacity is the whole cluster and each node in it.
type ClusterCapacity struct {
	CPU    Capacity `json:"cpu"`
	Memory Capacity `json:"memory"`
	Pods   Capacity `json:"pods"`

	Nodes []NodeCapacity `json:"nodes"`

	// Notice says what these numbers are, because "80% full" invites exactly
	// the wrong reading.
	Notice string `json:"notice"`
}

// Capacity reads how much room the cluster and each of its nodes has left.
//
// Two calls: every node and every pod. The pods are needed because a node does
// not report what has been reserved on it -- that is the scheduler's own
// arithmetic, and reproducing it is the only way to have the number at all.
func (c *Client) Capacity(ctx context.Context) (ClusterCapacity, error) {
	nodes, err := c.cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return ClusterCapacity{}, fmt.Errorf("kube: listing nodes: %w", err)
	}
	pods, err := c.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return ClusterCapacity{}, fmt.Errorf("kube: listing pods: %w", err)
	}

	type sums struct {
		cpu, memory *resource.Quantity
		count       int
	}
	perNode := map[string]*sums{}
	for _, pod := range pods.Items {
		// Only what the scheduler still counts. A Succeeded or Failed pod holds
		// no reservation, and counting them would report a node as full because
		// a CronJob ran a hundred times.
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		node := pod.Spec.NodeName
		if node == "" {
			// Unscheduled: it has been reserved nowhere, which is usually
			// BECAUSE there is no room. It is not attributed to a node.
			continue
		}
		into, ok := perNode[node]
		if !ok {
			into = &sums{
				cpu:    resource.NewQuantity(0, resource.DecimalSI),
				memory: resource.NewQuantity(0, resource.BinarySI),
			}
			perNode[node] = into
		}
		into.count++

		cpuReq, memReq, _, _ := podResources(pod)
		if q, err := resource.ParseQuantity(cpuReq); err == nil {
			into.cpu.Add(q)
		}
		if q, err := resource.ParseQuantity(memReq); err == nil {
			into.memory.Add(q)
		}
	}

	out := ClusterCapacity{
		Notice: "Requested is what the pods asked for, which is what the scheduler reserves and " +
			"what decides whether the next pod starts. It is not what anything is using: for " +
			"that a metrics-server has to be installed.",
	}

	totalCPU := resource.NewQuantity(0, resource.DecimalSI)
	totalCPUUsed := resource.NewQuantity(0, resource.DecimalSI)
	totalMemory := resource.NewQuantity(0, resource.BinarySI)
	totalMemoryUsed := resource.NewQuantity(0, resource.BinarySI)
	totalPods, totalPodsUsed := int64(0), int64(0)

	for _, node := range nodes.Items {
		used := perNode[node.Name]
		if used == nil {
			used = &sums{
				cpu:    resource.NewQuantity(0, resource.DecimalSI),
				memory: resource.NewQuantity(0, resource.BinarySI),
			}
		}

		row := NodeCapacity{
			Name:        node.Name,
			Cordoned:    node.Spec.Unschedulable,
			PodsRunning: used.count,
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				row.Ready = condition.Status == corev1.ConditionTrue
			}
		}

		row.CPU = capacityOf(node.Status.Allocatable, corev1.ResourceCPU, used.cpu)
		row.Memory = capacityOf(node.Status.Allocatable, corev1.ResourceMemory, used.memory)
		row.Pods = podCapacityOf(node.Status.Allocatable, used.count)

		// A cordoned or unready node's capacity is not the cluster's: nothing
		// new will be placed there, so counting it would report room that does
		// not exist. Its pods are still counted, because they are still using
		// it.
		if row.Ready && !row.Cordoned {
			addAllocatable(totalCPU, node.Status.Allocatable, corev1.ResourceCPU)
			addAllocatable(totalMemory, node.Status.Allocatable, corev1.ResourceMemory)
			if q, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
				totalPods += q.Value()
			}
		}
		totalCPUUsed.Add(*used.cpu)
		totalMemoryUsed.Add(*used.memory)
		totalPodsUsed += int64(used.count)

		out.Nodes = append(out.Nodes, row)
	}

	out.CPU = totals(totalCPU, totalCPUUsed)
	out.Memory = totals(totalMemory, totalMemoryUsed)
	out.Pods = Capacity{
		Allocatable: fmt.Sprintf("%d", totalPods),
		Requested:   fmt.Sprintf("%d", totalPodsUsed),
		Percent:     percent(totalPodsUsed*1000, totalPods*1000),
	}
	return out, nil
}

func capacityOf(list corev1.ResourceList, name corev1.ResourceName, used *resource.Quantity) Capacity {
	out := Capacity{Requested: used.String(), Percent: -1}
	q, ok := list[name]
	if !ok {
		// A node that has not reported its allocatable yet is not a node at 0%.
		return out
	}
	out.Allocatable = q.String()
	out.Percent = percent(used.MilliValue(), q.MilliValue())
	return out
}

func podCapacityOf(list corev1.ResourceList, running int) Capacity {
	out := Capacity{Requested: fmt.Sprintf("%d", running), Percent: -1}
	q, ok := list[corev1.ResourcePods]
	if !ok {
		return out
	}
	out.Allocatable = q.String()
	out.Percent = percent(int64(running)*1000, q.Value()*1000)
	return out
}

func addAllocatable(into *resource.Quantity, list corev1.ResourceList, name corev1.ResourceName) {
	if q, ok := list[name]; ok {
		into.Add(q)
	}
}

func totals(allocatable, used *resource.Quantity) Capacity {
	return Capacity{
		Allocatable: allocatable.String(),
		Requested:   used.String(),
		Percent:     percent(used.MilliValue(), allocatable.MilliValue()),
	}
}

// percent rounds, and answers -1 rather than 0 when there is nothing to divide
// by: a cluster whose nodes have not reported is not a cluster at 0% full.
func percent(used, total int64) int {
	if total <= 0 {
		return -1
	}
	return int((used*100 + total/2) / total)
}
