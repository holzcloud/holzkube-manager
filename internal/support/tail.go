package support

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/holzcloud/holzkube-manager/internal/audit"
)

// AuditTailFrom reads the newest audit records as JSONL.
//
// It goes through audit.Query rather than reading the day files, and that is
// not convenience: the records those files hold are already redacted by the
// allowlist on the way *in*, but Query is the one reader that this product's
// contract describes, and a bundle that opened the files itself would be a
// second path whose behaviour nobody keeps in step with the first.
//
// The records come back newest first, which is the order the query serves and
// the wrong order for a log somebody reads top to bottom. They are reversed
// here, once, so the file reads like the log it is.
func AuditTailFrom(dir string) func(ctx context.Context, n int) ([]byte, error) {
	return func(ctx context.Context, n int) ([]byte, error) {
		if n <= 0 || n > audit.MaxLimit {
			n = audit.MaxLimit
		}

		page, err := audit.Query(ctx, dir, audit.Filter{Limit: n})
		if err != nil {
			return nil, fmt.Errorf("support: read the audit tail: %w", err)
		}

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "# the newest %d audit record(s), oldest first. Parameters are redacted "+
			"by the allowlist in internal/audit/redact.go, which is fail-closed: a field not "+
			"explicitly permitted is written as %q.\n", len(page.Items), audit.RedactedMarker)

		for i := len(page.Items) - 1; i >= 0; i-- {
			line, merr := json.Marshal(page.Items[i])
			if merr != nil {
				// One record that does not encode must not cost the rest of
				// the tail: what is wanted here is the sequence, and a gap in
				// it that says so is more useful than no file.
				fmt.Fprintf(&buf, "{\"holzkube_manager_error\":%q}\n", merr.Error())
				continue
			}
			buf.Write(line)
			buf.WriteByte('\n')
		}
		return buf.Bytes(), nil
	}
}
