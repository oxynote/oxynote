package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/orlangure/gnomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// _victoriaMetricsMatcher selects every series of the samples.
	_victoriaMetricsMatcher = `{__name__=~"vm_test_.*"}`

	// _victoriaMetricsSeries is how many series the samples hold.
	_victoriaMetricsSeries = 3

	// _victoriaMetricsReadyInterval is how often the samples are checked
	// for being readable after they were imported.
	_victoriaMetricsReadyInterval = 200 * time.Millisecond
)

// _victoriaMetricsVersions are the VictoriaMetrics image tags the
// integration tests run against: the oldest supported release and the
// newest.
var _victoriaMetricsVersions = []string{"v1.130.0", "v1.156.0"}

// _victoriaMetricsAddrs maps a VictoriaMetrics version to the address of
// its container.
var _victoriaMetricsAddrs = map[string]string{}

// startVictoriaMetrics starts a VictoriaMetrics container of the given
// version holding the samples. It returns once every series can be read.
func startVictoriaMetrics(version string) (*gnomock.Container, error) {
	return gnomock.StartCustom(
		"docker.io/victoriametrics/victoria-metrics:"+version,
		gnomock.DefaultTCP(8428),
		gnomock.WithTimeout(3*time.Minute),
		gnomock.WithHealthCheck(func(ctx context.Context, c *gnomock.Container) error {
			_, err := containerRequest(ctx, http.MethodGet, "http://"+c.DefaultAddress()+"/health", "", nil)

			return err
		}),
		gnomock.WithInit(func(ctx context.Context, c *gnomock.Container) error {
			return importVictoriaMetricsSamples(ctx, c.DefaultAddress())
		}),
	)
}

// importVictoriaMetricsSamples imports a sample every 15s of the logs
// window for two request counters and a gauge, with their help and type.
// It returns once every series can be read.
func importVictoriaMetricsSamples(ctx context.Context, addr string) error {
	var body bytes.Buffer

	body.WriteString("# HELP vm_test_requests_total Requests served.\n# TYPE vm_test_requests_total counter\n")
	body.WriteString("# HELP vm_test_up Whether the job is up.\n# TYPE vm_test_up gauge\n")

	for i, t := 0, _logsWindow.From; t.Before(_logsWindow.To); i, t = i+1, t.Add(15*time.Second) {
		fmt.Fprintf(&body, "vm_test_requests_total{job=\"api\",code=\"200\"} %d %d\n", i*10, t.UnixMilli())
		fmt.Fprintf(&body, "vm_test_requests_total{job=\"api\",code=\"500\"} %d %d\n", i, t.UnixMilli())
		fmt.Fprintf(&body, "vm_test_up{job=\"api\"} 1 %d\n", t.UnixMilli())
	}

	if _, err := containerRequest(ctx, http.MethodPost, "http://"+addr+"/api/v1/import/prometheus", "", body.Bytes()); err != nil {
		return err
	}

	params := url.Values{
		"match[]": {_victoriaMetricsMatcher},
		"start":   {_logsWindow.From.Format(time.RFC3339)},
		"end":     {_logsWindow.To.Format(time.RFC3339)},
	}

	// imported samples become readable once VictoriaMetrics flushes them.
	return backoff.Retry(func() error {
		if _, err := containerRequest(ctx, http.MethodGet, "http://"+addr+"/internal/force_flush", "", nil); err != nil {
			return backoff.Permanent(err)
		}

		res, err := containerRequest(ctx, http.MethodGet, "http://"+addr+"/api/v1/series?"+params.Encode(), "", nil)
		if err != nil {
			return backoff.Permanent(err)
		}

		var series struct {
			Data []map[string]string `json:"data"`
		}

		if err := json.Unmarshal(res, &series); err != nil {
			return backoff.Permanent(err)
		}

		if len(series.Data) != _victoriaMetricsSeries {
			return fmt.Errorf("victoriametrics holds %d series, want %d", len(series.Data), _victoriaMetricsSeries)
		}

		return nil
	}, backoff.WithContext(backoff.NewConstantBackOff(_victoriaMetricsReadyInterval), ctx))
}

// prepVictoriaMetricsInput returns an input pointing at the container of
// the given version.
func prepVictoriaMetricsInput(t *testing.T, version string) *InputMock {
	t.Helper()

	return prepPrometheusInput(t, "http://"+_victoriaMetricsAddrs[version], nil)
}

func Test_NewVictoriaMetrics(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	vm := NewVictoriaMetrics(inp)
	require.NotNil(t, vm)
	assert.Same(t, inp, vm.inp)
}
