package explorerapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

const (
	performanceSamplesDefault = 31
	performanceWarmups        = 3
	performanceRounds         = 3
)

type performanceReceipt struct {
	SchemaVersion int                      `json:"schema_version"`
	HardwareClass string                   `json:"hardware_class"`
	GoVersion     string                   `json:"go_version"`
	GOMAXPROCS    int                      `json:"gomaxprocs"`
	Rounds        int                      `json:"rounds"`
	Samples       int                      `json:"samples"`
	Measurements  []performanceMeasurement `json:"measurements"`
}

type performanceMeasurement struct {
	Operation string `json:"operation"`
	Entities  int    `json:"entities"`
	P50NS     int64  `json:"p50_ns"`
	P95NS     int64  `json:"p95_ns"`
}

type performanceBaseline struct {
	SchemaVersion int                              `json:"schema_version"`
	HardwareClass string                           `json:"hardware_class"`
	Hardware      string                           `json:"hardware"`
	CapturedWith  string                           `json:"captured_with"`
	Tolerance     string                           `json:"tolerance"`
	Measurements  []performanceBaselineMeasurement `json:"measurements"`
}

type performanceBaselineMeasurement struct {
	Operation     string `json:"operation"`
	Entities      int    `json:"entities"`
	AcceptedP50NS int64  `json:"accepted_p50_ns"`
	AcceptedP95NS int64  `json:"accepted_p95_ns"`
	MaxP95NS      int64  `json:"max_p95_ns"`
}

// TestExplorerPerformanceRatchet is opt-in because wall-clock assertions are
// meaningful only on a named hardware class. The release gate sets both
// environment variables. Unknown hardware can still emit a receipt without
// pretending that one machine's latency budget applies to another machine.
func TestExplorerPerformanceRatchet(t *testing.T) {
	if os.Getenv("NICOS_CATALOG_PERF") != "1" {
		t.Skip("set NICOS_CATALOG_PERF=1 to measure Explorer latency")
	}

	samples := performanceSamples(t)
	receipt := performanceReceipt{
		SchemaVersion: 1,
		HardwareClass: performanceHardwareClass(),
		GoVersion:     runtime.Version(),
		GOMAXPROCS:    runtime.GOMAXPROCS(0),
		Rounds:        performanceRounds,
		Samples:       samples,
	}

	for _, size := range []int{500, 4_000, 10_000} {
		dataset := scaleDataset(size)
		service, err := NewService(dataset)
		if err != nil {
			t.Fatalf("build %d-entity service: %v", size, err)
		}

		operations := []struct {
			name string
			run  func() error
		}{
			{
				name: "load",
				run: func() error {
					_, err := NewService(dataset)
					return err
				},
			},
			{
				name: "list",
				run: func() error {
					_, _, err := service.List(ListOptions{Limit: 50, Sort: "id"})
					return err
				},
			},
			{
				name: "search",
				run: func() error {
					_, _, err := service.Search(SearchOptions{Query: "bounded catalog", Limit: 50})
					return err
				},
			},
			{
				name: "graph",
				run: func() error {
					_, _, err := service.Graph(GraphOptions{
						Mode:    explorercontract.GraphAggregate,
						GroupBy: explorercontract.GroupKind,
					})
					return err
				},
			},
		}

		for _, operation := range operations {
			p50, p95 := measurePerformance(t, samples, operation.run)
			receipt.Measurements = append(receipt.Measurements, performanceMeasurement{
				Operation: operation.name,
				Entities:  size,
				P50NS:     p50.Nanoseconds(),
				P95NS:     p95.Nanoseconds(),
			})
		}
	}

	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("EXPLORER_PERFORMANCE_RECEIPT=%s", encoded)

	baseline, found := readPerformanceBaseline(t, receipt.HardwareClass)
	if !found {
		if os.Getenv("NICOS_CATALOG_PERF_REQUIRED") == "1" {
			t.Fatalf("no performance baseline for hardware class %q", receipt.HardwareClass)
		}
		t.Logf("no ratchet applies to hardware class %q", receipt.HardwareClass)
		return
	}
	applyPerformanceRatchet(t, baseline, receipt)
}

func performanceSamples(t *testing.T) int {
	t.Helper()
	raw := os.Getenv("NICOS_CATALOG_PERF_SAMPLES")
	if raw == "" {
		return performanceSamplesDefault
	}
	samples, err := strconv.Atoi(raw)
	if err != nil || samples < 10 || samples > 200 {
		t.Fatalf("NICOS_CATALOG_PERF_SAMPLES must be between 10 and 200, got %q", raw)
	}
	return samples
}

func performanceHardwareClass() string {
	return fmt.Sprintf("%s-%s-%dcpu", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

func measurePerformance(t *testing.T, samples int, operation func() error) (time.Duration, time.Duration) {
	t.Helper()
	p50s := make([]time.Duration, performanceRounds)
	p95s := make([]time.Duration, performanceRounds)
	for round := range performanceRounds {
		for range performanceWarmups {
			if err := operation(); err != nil {
				t.Fatal(err)
			}
		}

		durations := make([]time.Duration, samples)
		for i := range durations {
			started := time.Now()
			if err := operation(); err != nil {
				t.Fatal(err)
			}
			durations[i] = time.Since(started)
		}
		slices.Sort(durations)
		p50s[round] = durations[percentileIndex(len(durations), 50)]
		p95s[round] = durations[percentileIndex(len(durations), 95)]
	}
	slices.Sort(p50s)
	slices.Sort(p95s)
	return p50s[len(p50s)/2], p95s[len(p95s)/2]
}

func percentileIndex(size, percentile int) int {
	// Nearest-rank percentile with a zero-based index.
	index := (size*percentile + 99) / 100
	if index < 1 {
		return 0
	}
	if index > size {
		return size - 1
	}
	return index - 1
}

func readPerformanceBaseline(t *testing.T, hardwareClass string) (performanceBaseline, bool) {
	t.Helper()
	path := filepath.Join("testdata", "performance", hardwareClass+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return performanceBaseline{}, false
	}
	if err != nil {
		t.Fatalf("read performance baseline: %v", err)
	}
	var baseline performanceBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatalf("decode performance baseline: %v", err)
	}
	if baseline.SchemaVersion != 1 || baseline.HardwareClass != hardwareClass {
		t.Fatalf("performance baseline identity mismatch: %+v", baseline)
	}
	return baseline, true
}

func applyPerformanceRatchet(t *testing.T, baseline performanceBaseline, receipt performanceReceipt) {
	t.Helper()
	wanted := make(map[string]performanceBaselineMeasurement, len(baseline.Measurements))
	for _, measurement := range baseline.Measurements {
		key := fmt.Sprintf("%s/%d", measurement.Operation, measurement.Entities)
		if measurement.AcceptedP50NS <= 0 || measurement.AcceptedP95NS <= 0 || measurement.MaxP95NS < measurement.AcceptedP95NS {
			t.Fatalf("invalid performance baseline %s: %+v", key, measurement)
		}
		if _, duplicate := wanted[key]; duplicate {
			t.Fatalf("duplicate performance baseline %s", key)
		}
		wanted[key] = measurement
	}

	for _, measurement := range receipt.Measurements {
		key := fmt.Sprintf("%s/%d", measurement.Operation, measurement.Entities)
		budget, ok := wanted[key]
		if !ok {
			t.Errorf("performance baseline is missing %s", key)
			continue
		}
		delete(wanted, key)
		if measurement.P95NS > budget.MaxP95NS {
			t.Errorf("%s p95 %s exceeds ratchet %s (accepted p95 %s)",
				key,
				time.Duration(measurement.P95NS),
				time.Duration(budget.MaxP95NS),
				time.Duration(budget.AcceptedP95NS),
			)
		}
	}
	for key := range wanted {
		t.Errorf("performance receipt is missing %s", key)
	}
}
