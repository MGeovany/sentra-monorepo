package cli

import (
	"net/http"
	"strings"
)

func responseTrace(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	for _, k := range []string{"X-Request-Id", "X-Cloud-Trace-Context", "traceparent", "X-Amzn-Trace-Id"} {
		v := strings.TrimSpace(resp.Header.Get(k))
		if v != "" {
			return k + "=" + oneLine(v)
		}
	}
	return ""
}
