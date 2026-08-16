package metricsquery

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"fuze-ai-paas/backend/internal/ports"
)

func fakeProm(t *testing.T, handler http.HandlerFunc) (*Prometheus, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := NewPrometheus(PrometheusConfig{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("new prometheus: %v", err)
	}
	return p, srv
}

func TestPrometheusQueryRangeParsesMatrix(t *testing.T) {
	p, _ := fakeProm(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{
				"resultType":"matrix",
				"result":[
					{"metric":{"gpu":"0"},"values":[[1700000000,"0.5"],[1700000060,"0.8"]]},
					{"metric":{"gpu":"1"},"values":[[1700000000,"0.3"]]}
				]
			}
		}`))
	})

	series, err := p.QueryRange(ports.MetricQuery{Query: "gpu_util", Step: 60})
	if err != nil {
		t.Fatalf("query range: %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("expected 2 series, got %d", len(series))
	}
	if series[0].Labels["gpu"] != "0" {
		t.Fatalf("expected label gpu=0, got %s", series[0].Labels["gpu"])
	}
	if len(series[0].Samples) != 2 {
		t.Fatalf("expected 2 samples in first series, got %d", len(series[0].Samples))
	}
	if series[0].Samples[1].Value != 0.8 {
		t.Fatalf("expected second sample value 0.8, got %v", series[0].Samples[1].Value)
	}
}

func TestPrometheusQueryLatestPicksLast(t *testing.T) {
	p, _ := fakeProm(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{"resultType":"vector","result":[
				{"metric":{"pod":"w0"},"value":[1700000100,"0.99"]}
			]}
		}`))
	})

	sample, err := p.QueryLatest(ports.MetricQuery{Query: "gpu_util"})
	if err != nil {
		t.Fatalf("query latest: %v", err)
	}
	if sample == nil {
		t.Fatalf("expected a sample, got nil")
	}
	if sample.Value != 0.99 {
		t.Fatalf("expected latest value 0.99, got %v", sample.Value)
	}
}

func TestPrometheusErrorStatus(t *testing.T) {
	p, _ := fakeProm(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := p.QueryRange(ports.MetricQuery{Query: "x"})
	if err == nil {
		t.Fatalf("expected error on 503")
	}
}

func TestPrometheusRequiresBaseURL(t *testing.T) {
	if _, err := NewPrometheus(PrometheusConfig{}); err == nil {
		t.Fatalf("expected error when base url empty")
	}
}

func TestNoopReturnsEmpty(t *testing.T) {
	n := NewNoop()
	series, err := n.QueryRange(ports.MetricQuery{Query: "x"})
	if err != nil || len(series) != 0 {
		t.Fatalf("noop should return empty series, got %v err %v", series, err)
	}
	sample, err := n.QueryLatest(ports.MetricQuery{Query: "x"})
	if err != nil || sample != nil {
		t.Fatalf("noop should return nil sample, got %v err %v", sample, err)
	}
}

func TestMetricQueryInjectionRejected(t *testing.T) {
	cases := []ports.MetricQuery{
		{Query: "up{foo=\"bar\"} or up"},                 
		{Query: "up or node_cpu"},                       
		{Query: "rate(x[5m]) unless y"},                 
		{Query: "foo | bar"},                            
		{Query: "foo; drop"},                            
		{Query: "foo{job_id=\"x\"}", JobID: "job-1"},    
		{Query: "gpu_util", JobID: "bad job id!!"},      
		{Query: ""},                                     
	}
	for _, q := range cases {
		_, err := fakePromQueryLatest(t, q)
		if err == nil {
			t.Fatalf("expected injection query %q (job=%q) to be rejected", q.Query, q.JobID)
		}
	}
}

func TestMetricQueryValidAccepted(t *testing.T) {
	cases := []ports.MetricQuery{
		{Query: "gpu_util"},
		{Query: "training_loss", JobID: "job-123"},
		{Query: "gpu_util_percent", JobID: "exp_a-1"},
	}
	for _, q := range cases {
		if _, err := fakePromQueryLatest(t, q); err != nil {
			t.Fatalf("valid query %q (job=%q) unexpectedly rejected: %v", q.Query, q.JobID, err)
		}
	}
}

func fakePromQueryLatest(t *testing.T, q ports.MetricQuery) (*ports.MetricSample, error) {
	t.Helper()
	p, _ := fakeProm(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"pod":"w0"},"value":[1700000100,"0.99"]}]}}`))
	})
	return p.QueryLatest(q)
}

func TestMetricQueryLabelsAppended(t *testing.T) {
	var gotQuery string
	p, _ := fakeProm(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	})
	q := ports.MetricQuery{
		Query:  "edge_feature_mean",
		Labels: map[string]string{"tenant_id": "t1", "deployment_id": "dep-1"},
	}
	if _, err := p.QueryLatest(q); err != nil {
		t.Fatalf("query latest with labels: %v", err)
	}
	want := `edge_feature_mean{deployment_id="dep-1",tenant_id="t1"}`
	if gotQuery != want {
		t.Fatalf("expected query %q, got %q", want, gotQuery)
	}
}

func TestMetricQueryLabelsInjectionRejected(t *testing.T) {
	cases := []ports.MetricQuery{
		{Query: "x", Labels: map[string]string{"bad key": "v"}},
		{Query: "x", Labels: map[string]string{"k": "v\""}},
		{Query: "x", Labels: map[string]string{"k;drop": "v"}},
	}
	for _, q := range cases {
		if _, err := fakePromQueryLatest(t, q); err == nil {
			t.Fatalf("expected label %v to be rejected", q.Labels)
		}
	}
}