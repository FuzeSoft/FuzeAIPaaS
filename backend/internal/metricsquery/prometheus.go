package metricsquery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/ports"
)

type Prometheus struct {
	baseURL string
	client  *http.Client
}

type PrometheusConfig struct {
	
	BaseURL string
	
	Timeout time.Duration
}

func NewPrometheus(cfg PrometheusConfig) (*Prometheus, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("metricsquery: prometheus base url is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Prometheus{
		baseURL: cfg.BaseURL,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
			},
		},
	}, nil
}

var jobIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validateMetricQuery(q ports.MetricQuery) error {
	const maxLen = 2000
	if strings.TrimSpace(q.Query) == "" {
		return fmt.Errorf("metric query is empty")
	}
	if len(q.Query) > maxLen {
		return fmt.Errorf("metric query too long (max %d chars)", maxLen)
	}
	if strings.ContainsAny(q.Query, "{()};\t\n\r`|") {
		return fmt.Errorf("metric query contains disallowed characters")
	}
	lower := strings.ToLower(q.Query)
	for _, op := range []string{" or ", " and ", " unless ", "or(", "and(", "unless("} {
		if strings.Contains(lower, op) {
			return fmt.Errorf("metric query contains disallowed operator")
		}
	}
	if q.JobID != "" {
		if !jobIDPattern.MatchString(q.JobID) {
			return fmt.Errorf("invalid job_id format")
		}
		
		if strings.ContainsAny(q.Query, "{}") {
			return fmt.Errorf("metric query must not contain selectors when job_id is set")
		}
	}
	
	for k, v := range q.Labels {
		if !jobIDPattern.MatchString(k) || !jobIDPattern.MatchString(v) {
			return fmt.Errorf("invalid metric label key/value: %q=%q", k, v)
		}
	}
	return nil
}

func (p *Prometheus) QueryRange(q ports.MetricQuery) ([]ports.MetricSeries, error) {
	if err := validateMetricQuery(q); err != nil {
		return nil, fmt.Errorf("metricsquery: %w", err)
	}
	
	start, end := q.Start, q.End
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	if start == 0 {
		start = end - 60*60*1000
	}
	step := q.Step
	if step <= 0 {
		step = 60
	}

	expr := p.resolveQuery(q)
	u := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%s&end=%s&step=%s",
		p.baseURL,
		url.QueryEscape(expr),
		strconv.FormatInt(toSeconds(start), 10),
		strconv.FormatInt(toSeconds(end), 10),
		strconv.Itoa(step),
	)

	data, err := p.doQuery(u)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *Prometheus) QueryLatest(q ports.MetricQuery) (*ports.MetricSample, error) {
	if err := validateMetricQuery(q); err != nil {
		return nil, fmt.Errorf("metricsquery: %w", err)
	}
	expr := p.resolveQuery(q)
	u := fmt.Sprintf("%s/api/v1/query?query=%s", p.baseURL, url.QueryEscape(expr))
	data, err := p.doQuery(u)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data[0].Samples) == 0 {
		return nil, nil
	}
	samples := data[0].Samples
	return &samples[len(samples)-1], nil
}

func (p *Prometheus) resolveQuery(q ports.MetricQuery) string {
	sel := ""
	added := false
	if q.JobID != "" {
		sel += fmt.Sprintf("job_id=%q", q.JobID)
		added = true
	}
	
	keys := make([]string, 0, len(q.Labels))
	for k := range q.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if added {
			sel += ","
		}
		sel += fmt.Sprintf("%s=%q", k, q.Labels[k])
		added = true
	}
	if !added {
		return q.Query
	}
	return fmt.Sprintf("%s{%s}", q.Query, sel)
}

func (p *Prometheus) doQuery(u string) ([]ports.MetricSeries, error) {
	resp, err := p.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("metricsquery: prometheus request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metricsquery: prometheus returned status %d", resp.StatusCode)
	}

	var pr promResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("metricsquery: decode prometheus response: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("metricsquery: prometheus query status %q: %s", pr.Status, pr.Error)
	}

	return parsePromResult(pr.Data.Result), nil
}

func toSeconds(ms int64) int64 {
	return ms / 1000
}

func parsePromResult(results []promResult) []ports.MetricSeries {
	out := make([]ports.MetricSeries, 0, len(results))
	for _, r := range results {
		series := ports.MetricSeries{Labels: r.Metric}
		if len(r.Values) > 0 {
			
			series.Samples = make([]ports.MetricSample, 0, len(r.Values))
			for _, v := range r.Values {
				if len(v) < 2 {
					continue
				}
				series.Samples = append(series.Samples, sampleOf(v))
			}
		} else if len(r.Value) >= 2 {
			
			series.Samples = []ports.MetricSample{sampleOf(r.Value)}
		}
		out = append(out, series)
	}
	return out
}

func (p *Prometheus) Alerts() ([]ports.ActiveAlert, error) {
	u := fmt.Sprintf("%s/api/v1/alerts", p.baseURL)
	resp, err := p.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("metricsquery: prometheus alerts request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metricsquery: prometheus alerts returned status %d", resp.StatusCode)
	}
	var pr promAlertsResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("metricsquery: decode prometheus alerts response: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("metricsquery: prometheus alerts status %q: %s", pr.Status, pr.Error)
	}
	out := make([]ports.ActiveAlert, 0, len(pr.Data.Alerts))
	for _, a := range pr.Data.Alerts {
		labels := map[string]string{}
		for k, v := range a.Labels {
			labels[k] = v
		}
		annotations := map[string]string{}
		for k, v := range a.Annotations {
			annotations[k] = v
		}
		out = append(out, ports.ActiveAlert{
			Fingerprint: a.Fingerprint,
			RuleName:    a.Labels["alertname"],
			State:       a.State,
			Severity:    a.Labels["severity"],
			Labels:      labels,
			Annotations: annotations,
			ActiveAt:    int64(a.ActiveAt * 1000),
			Value:       a.Value,
		})
	}
	return out, nil
}

type promResponse struct {
	Status string       `json:"status"`
	Data   promData     `json:"data"`
	Error  string       `json:"error,omitempty"`
}

type promData struct {
	ResultType string       `json:"resultType"`
	Result     []promResult `json:"result"`
}

type promResult struct {
	Metric map[string]string `json:"metric"`
	
	Values [][]interface{} `json:"values"`
	
	Value []interface{} `json:"value"`
}

func sampleOf(v []interface{}) ports.MetricSample {
	ts, _ := v[0].(float64)
	val, _ := v[1].(string)
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		f = 0
	}
	return ports.MetricSample{Timestamp: int64(ts * 1000), Value: f}
}

type promAlertsResponse struct {
	Status string           `json:"status"`
	Data   promAlertsData   `json:"data"`
	Error  string           `json:"error,omitempty"`
}

type promAlertsData struct {
	Alerts []promAlert `json:"alerts"`
}

type promAlert struct {
	Fingerprint  string            `json:"fingerprint"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	State        string            `json:"state"`
	ActiveAt     float64           `json:"activeAt"`
	Value        string            `json:"value"`
}