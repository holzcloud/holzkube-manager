package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reaching a workload through the cluster (milestone v1.17, slice 6).
//
// # What this is, and what it deliberately is not
//
// It FETCHES from a service. It does not HOST one. The difference decides the
// whole design, and it is the difference between a useful tool and a hole.
//
// A proxy that piped a service's response back as the service described it --
// its content type, its HTML, its scripts -- would be serving that workload's
// markup from this daemon's own origin, which is the origin holding the
// operator's session cookie. Any pod in the cluster could then script this
// product's interface. So the body comes back as TEXT, whatever the service
// claimed it was, and the screen shows it as text. That makes /healthz,
// /readyz, /metrics and a JSON API answer reachable -- which is what an operator
// actually wants from here -- and makes hosting a workload's web interface
// something this product does not do.
//
// # It goes through the API server's own proxy
//
// Not through a port-forward tunnel opened by this daemon. The API server has a
// proxy subresource for services; using it means the cluster's own
// authorisation decides whether this identity may reach that service, the
// connection is the one this product already has, and no new listener exists
// anywhere. A port-forward path through the daemon would have been a second
// network path into the cluster, owned by this process.
//
// # GET only
//
// The identity this product holds is powerful. A proxy that forwarded any method
// would let anybody with an operator session drive any in-cluster API -- an
// unauthenticated admin endpoint on some pod included -- with this product's
// credentials, and the audit log would record "proxy" rather than what was done.
// Reading is the use case; writing through here is not offered.

// ErrProxyPathRefused reports a path this product will not forward.
var ErrProxyPathRefused = errors.New("kube: that is not a path this proxy will forward")

// MaxProxyBytes bounds one proxied response.
//
// A service can answer with a gigabyte, and this process would hold it. A
// truncated answer is reported as truncated rather than silently cut, because
// half of a metrics page that looked whole would be read as a complete one.
const MaxProxyBytes = 1 << 20

// ProxyBudget bounds one proxied request.
//
// Shorter than CallBudget on purpose: behind this call is not the API server
// answering out of etcd but an arbitrary workload, and a pod that accepts a
// connection and never answers is the ordinary failure of a broken workload
// rather than an exceptional one.
const ProxyBudget = 20 * time.Second

// Service is one service, as somebody choosing what to reach needs it.
type Service struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// Type is ClusterIP, NodePort, LoadBalancer or ExternalName. It is here
	// because it answers "why can I not reach this from outside", which is the
	// question that brings somebody to this screen.
	Type string `json:"type"`

	ClusterIP string `json:"cluster_ip"`

	// Ports are the ports the service exposes, as the proxy needs them named:
	// the number, and the name if it has one.
	Ports []ServicePort `json:"ports"`
}

// ServicePort is one port of a service.
type ServicePort struct {
	Name     string `json:"name"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
}

// ProxyResponse is what a service answered.
type ProxyResponse struct {
	// Status is the workload's own HTTP status. It is carried rather than
	// flattened into an error: a 503 from a health endpoint is the answer
	// somebody came here for, not a failure of this product.
	Status int `json:"status"`

	// Body is the response, as TEXT. Whatever content type the service claimed
	// is deliberately not carried: this product does not serve a workload's
	// markup from its own origin.
	Body string `json:"body"`

	// Truncated is true when the service sent more than MaxProxyBytes.
	Truncated bool `json:"truncated"`
}

// Services lists services, in one namespace or across all of them.
func (c *Client) Services(ctx context.Context, namespace string) ([]Service, error) {
	list, err := c.cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing services: %w", err)
	}

	out := make([]Service, 0, len(list.Items))
	for _, s := range list.Items {
		row := Service{
			Namespace: s.Namespace,
			Name:      s.Name,
			Type:      string(s.Spec.Type),
			ClusterIP: s.Spec.ClusterIP,
		}
		for _, p := range s.Spec.Ports {
			row.Ports = append(row.Ports, ServicePort{
				Name: p.Name, Port: p.Port, Protocol: string(p.Protocol),
			})
		}
		out = append(out, row)
	}
	return out, nil
}

// ProxyGet fetches one path from a service, through the API server's proxy.
//
// The port is given as the service's port number or its port name, the way the
// subresource wants it. The scheme is http: https through the proxy would need
// this product to decide what to do about the workload's certificate, and
// deciding that quietly is worse than not offering it.
func (c *Client) ProxyGet(
	ctx context.Context, namespace, service, port, urlPath string,
) (ProxyResponse, error) {
	target, err := proxyTarget(service, port)
	if err != nil {
		return ProxyResponse{}, err
	}
	suffix, err := proxyPath(urlPath)
	if err != nil {
		return ProxyResponse{}, err
	}

	// Confirmed before it is asked for, so that "no such service" is a clear
	// answer rather than a 404 that could equally mean the path does not exist
	// on a service that does.
	if _, err := c.cs.CoreV1().Services(namespace).Get(
		ctx, service, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return ProxyResponse{}, fmt.Errorf("%w: service %s/%s", ErrNoSuchWorkload, namespace, service)
		}
		return ProxyResponse{}, fmt.Errorf("kube: reading service %s/%s: %w", namespace, service, err)
	}

	request := c.cs.CoreV1().RESTClient().Get().
		Namespace(namespace).
		Resource("services").
		Name(target).
		SubResource("proxy").
		Suffix(suffix)

	stream, err := request.Stream(ctx)
	if err != nil {
		// A status error from the API server carries the workload's own code
		// when the workload answered with one, and that is worth keeping: a
		// 503 from a health endpoint is the answer, not a failure.
		var status apierrors.APIStatus
		if errors.As(err, &status) && status.Status().Code >= 400 {
			return ProxyResponse{
				Status: int(status.Status().Code),
				Body:   status.Status().Message,
			}, nil
		}
		return ProxyResponse{}, fmt.Errorf("kube: reaching %s/%s%s: %w", namespace, target, suffix, err)
	}
	defer func() { _ = stream.Close() }()

	// One byte more than the cap, so that hitting it exactly can be told from
	// exceeding it.
	body, err := io.ReadAll(io.LimitReader(stream, MaxProxyBytes+1))
	if err != nil {
		return ProxyResponse{}, fmt.Errorf("kube: reading from %s/%s: %w", namespace, target, err)
	}

	response := ProxyResponse{Status: 200}
	if len(body) > MaxProxyBytes {
		body = body[:MaxProxyBytes]
		response.Truncated = true
	}
	response.Body = string(body)
	return response, nil
}

// proxyTarget builds the "service:port" the proxy subresource is addressed by.
func proxyTarget(service, port string) (string, error) {
	if service == "" {
		return "", fmt.Errorf("%w: no service was named", ErrProxyPathRefused)
	}
	if port == "" {
		return "", fmt.Errorf("%w: no port was named. A service can expose several, so this "+
			"product does not pick one", ErrProxyPathRefused)
	}

	// A number, or a port name. Both are legal in the subresource. Validating
	// them here keeps a colon or a slash out of the middle of the URL this
	// builds: the API server parses "name:port" itself, so a port carrying a
	// second colon makes it parse something other than what was meant.
	if n, err := strconv.Atoi(port); err == nil {
		if n < 1 || n > 65535 {
			return "", fmt.Errorf("%w: %d is not a port", ErrProxyPathRefused, n)
		}
		return service + ":" + port, nil
	}
	for _, r := range port {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return "", fmt.Errorf("%w: %q is neither a port number nor a port name",
				ErrProxyPathRefused, port)
		}
	}
	return service + ":" + port, nil
}

// proxyPath normalises the path and refuses one that tries to leave it.
func proxyPath(urlPath string) (string, error) {
	if urlPath == "" {
		return "/", nil
	}
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	if strings.ContainsAny(urlPath, "\n\r") {
		return "", fmt.Errorf("%w: a path cannot contain a line break", ErrProxyPathRefused)
	}

	// The check is on the path as WRITTEN, not on the cleaned one.
	//
	// What it does NOT prevent was measured rather than assumed: with the check
	// removed, "/healthz/../../../api/v1/secrets" arrives at the service as
	// "/api/v1/secrets" -- `path.Clean` collapses it and client-go escapes each
	// segment, so it stays inside the proxy subresource and does not reach the
	// API server's own resources. The scare story is not the reason.
	//
	// The reason is quieter and still worth a refusal: a path with ".." in it is
	// silently REWRITTEN into a different path, and the operator is shown the
	// answer to a question they did not ask. Refusing says so instead.
	for _, part := range strings.Split(urlPath, "/") {
		if part == ".." {
			return "", fmt.Errorf("%w: %q would be rewritten to something else before it was "+
				"sent, and you would be shown that answer instead", ErrProxyPathRefused, urlPath)
		}
	}
	return path.Clean(urlPath), nil
}
