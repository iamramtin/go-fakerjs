// Package conformance is the engine-agnostic golden-data test harness.
//
// The same checks run against the native wasm2go engine and against the third-party baselines in the
// nested bench module, so every engine is judged by identical expectations. The package is ordinary
// (non-test) code only so that the bench module can import it; it uses nothing outside the standard library.
package conformance

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RefDate is the default reference date the golden data was generated with.
var RefDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Engine is the surface every implementation under test exposes.
type Engine interface {
	Generate(method string, arg any) (string, error)
	GenerateN(method string, arg any, n int) ([]string, error)
	Close()
}

// Factory builds an engine whose sequence starts at seed with the golden reference date applied.
type Factory struct {
	Name string
	New  func(seed int64, refDate time.Time) (Engine, error)
	// Strict makes unlisted divergences and stale known_divergences entries test failures. Baselines run
	// non-strict: they are measured, not held to the native engine's divergence catalogue.
	Strict bool
}

// Classed is implemented by errors that carry a coarse, engine-independent class.
type Classed interface{ ErrorClass() string }

// Error classes shared by the golden data and every engine.
const (
	ClassInvalidMethod = "invalid-method"
	ClassUnknownMethod = "unknown-method"
	ClassJSException   = "js-exception"
)

// ClassOf maps an engine error to its class. Errors that do not implement [Classed] are classified by
// message so third-party baselines can be measured without modification.
func ClassOf(err error) string {
	var c Classed
	if errors.As(err, &c) {
		return c.ErrorClass()
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid faker method format"), strings.Contains(msg, "cannot be empty"):
		return ClassInvalidMethod
	case strings.Contains(msg, "unknown faker"):
		return ClassUnknownMethod
	}

	return ClassJSException
}

// Result is one recorded call outcome: a String()-coerced value or an error class.
type Result struct {
	Value string
	Err   string // non-empty means an error of this class was expected / observed
}

func (r *Result) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		r.Err = ""
		return json.Unmarshal(b, &r.Value)
	}

	var o struct {
		E string `json:"e"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}

	r.Value, r.Err = "", o.E

	return nil
}

func (r Result) String() string {
	if r.Err != "" {
		return "error(" + r.Err + ")"
	}

	return strconv.Quote(r.Value)
}

// CallSpec is one recorded call, repeated N times (default 1).
type CallSpec struct {
	M string `json:"m"`
	A any    `json:"a"`
	N int    `json:"n"`
}

// Case is one golden case: a fresh engine at Seed performs Calls in order and must produce R.
type Case struct {
	ID    string     `json:"id"`
	Kind  string     `json:"kind"`
	Seed  string     `json:"seed"`
	Calls []CallSpec `json:"calls"`
	R     []Result   `json:"r"`
}

// Method returns the module.method of the first call, used to group per-method cases.
func (c *Case) Method() string {
	if len(c.Calls) == 0 {
		return ""
	}

	return c.Calls[0].M
}

// Module returns the faker module of the first call ("" when the method is malformed).
func (c *Case) Module() string {
	m, _, _ := strings.Cut(c.Method(), ".")
	return m
}

// Root is the repository root, located relative to this source file.
func Root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

// GoldenDir is golden/testdata.
func GoldenDir() string { return filepath.Join(Root(), "golden", "testdata") }

// ReportsDir is the git-ignored reports directory, created on demand.
func ReportsDir() string {
	d := filepath.Join(Root(), "reports")
	_ = os.MkdirAll(d, 0o755)

	return d
}

// Kinds of golden file, matching golden/testdata/<kind>.jsonl.gz.
const (
	PerMethod   = "permethod"
	SeedEdge    = "seededge"
	Interleaved = "interleaved"
	Workload22  = "workload22"
	Coercion    = "coercion"
	Errors      = "errors"
)

// Load reads every case of one golden file.
func Load(kind string) ([]Case, error) {
	f, err := os.Open(filepath.Join(GoldenDir(), kind+".jsonl.gz"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()

	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 256<<20)

	var out []Case

	for sc.Scan() {
		var c Case
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, fmt.Errorf("%s: %w", kind, err)
		}

		out = append(out, c)
	}

	return out, sc.Err()
}

// Meta is golden/testdata/meta.json.
type Meta struct {
	FakerVersion string   `json:"fakerVersion"`
	O1EqualsO2   bool     `json:"o1EqualsO2"`
	Methods      int      `json:"methods"`
	Workload22   []string `json:"workload22"`
}

// LoadMeta reads meta.json.
func LoadMeta() (Meta, error) {
	var m Meta

	b, err := os.ReadFile(filepath.Join(GoldenDir(), "meta.json"))
	if err != nil {
		return m, err
	}

	return m, json.Unmarshal(b, &m)
}

// API is golden/api.json.
type API struct {
	FakerVersion string              `json:"fakerVersion"`
	Total        int                 `json:"total"`
	Modules      map[string][]string `json:"modules"`
}

// LoadAPI reads api.json.
func LoadAPI() (API, error) {
	var a API

	b, err := os.ReadFile(filepath.Join(Root(), "golden", "api.json"))
	if err != nil {
		return a, err
	}

	return a, json.Unmarshal(b, &a)
}

// Run executes one case on a fresh engine and returns the observed results in order.
func Run(f Factory, c *Case) ([]Result, error) {
	seed, err := strconv.ParseInt(c.Seed, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("case %s: bad seed %q", c.ID, c.Seed)
	}

	e, err := f.New(seed, RefDate)
	if err != nil {
		return nil, err
	}
	defer e.Close()

	var out []Result

	for _, cs := range c.Calls {
		n := cs.N
		if n == 0 {
			n = 1
		}

		for i := 0; i < n; i++ {
			v, err := e.Generate(cs.M, cs.A)
			if err != nil {
				out = append(out, Result{Err: ClassOf(err)})
				continue
			}

			out = append(out, Result{Value: v})
		}
	}

	return out, nil
}

// Disposition of one call or one case.
type Disposition int

const (
	Exact Disposition = iota
	Normalized
	Divergent
)

func (d Disposition) String() string { return [...]string{"EXACT", "NORMALIZED", "DIVERGENT"}[d] }

// dateTZName strips the "(Coordinated Universal Time)" style suffix after a GMT offset. QuickJS
// formats Date.prototype.toString without it; V8 appends the time zone display name.
var dateTZName = regexp.MustCompile(`(GMT[+-]\d{4}) \([^)]*\)`)

// Normalize applies the declared normalizers. Each one must be named in FINDINGS.md.
func Normalize(s string) string { return dateTZName.ReplaceAllString(s, "$1") }

// CompareOne classifies a single call.
func CompareOne(exp, act Result) Disposition {
	switch {
	case exp.Err != "" || act.Err != "":
		if exp.Err == act.Err {
			return Exact
		}

		return Divergent
	case exp.Value == act.Value:
		return Exact
	case Normalize(exp.Value) == Normalize(act.Value):
		return Normalized
	}

	return Divergent
}

// Outcome is the verdict for one case.
type Outcome struct {
	Case        *Case
	Disposition Disposition
	// First divergent call, valid when Disposition == Divergent.
	Index         int
	Expected, Got Result
	Calls         []string // the call sequence around Index, for diagnostics
}

// Judge compares actual results with the case's expected results.
func Judge(c *Case, got []Result) Outcome {
	o := Outcome{Case: c, Disposition: Exact}
	if len(got) != len(c.R) {
		o.Disposition = Divergent
		o.Index = min(len(got), len(c.R))

		return o
	}

	for i := range got {
		switch CompareOne(c.R[i], got[i]) {
		case Exact:
			// An exact call leaves the case's disposition as it is.
		case Normalized:
			if o.Disposition == Exact {
				o.Disposition = Normalized
			}
		case Divergent:
			o.Disposition = Divergent
			o.Index, o.Expected, o.Got = i, c.R[i], got[i]
			o.Calls = describeAround(c, i)

			return o
		}
	}

	return o
}

// describeAround returns "method(arg) = result" for the call at index and the three before it.
func describeAround(c *Case, index int) []string {
	flat := make([]string, 0, len(c.R))
	for _, cs := range c.Calls {
		n := cs.N
		if n == 0 {
			n = 1
		}

		a, _ := json.Marshal(cs.A)
		for i := 0; i < n; i++ {
			flat = append(flat, cs.M+"("+strings.TrimSuffix(string(a), "null")+")")
		}
	}

	var out []string
	for i := max(0, index-3); i <= index && i < len(flat); i++ {
		out = append(out, fmt.Sprintf("#%d %s expected %s", i, flat[i], c.R[i]))
	}

	return out
}

// Divergence is one entry of golden/known_divergences.yaml.
type Divergence struct {
	Method   string
	Variant  string // variant index or "*"
	Seed     string // seed or "*"
	Reason   string
	Category string
}

// loadYAMLList parses the small YAML subset the golden metadata files use: a top-level list of flat maps
// with scalar values, "#" comments, optional quotes, or an empty "[]".
func loadYAMLList(path string) ([]map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var (
		out []map[string]string
		cur map[string]string
	)

	for line := range strings.SplitSeq(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || trim == "[]" || trim == "---" {
			continue
		}

		if strings.HasPrefix(trim, "- ") {
			cur = map[string]string{}
			out = append(out, cur)
			trim = strings.TrimPrefix(trim, "- ")
		}

		k, v, ok := strings.Cut(trim, ":")
		if cur == nil || !ok {
			return nil, fmt.Errorf("%s: unexpected line %q", filepath.Base(path), line)
		}

		cur[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}

	return out, nil
}

// LoadDivergences parses golden/known_divergences.yaml.
func LoadDivergences() ([]Divergence, error) {
	rows, err := loadYAMLList(filepath.Join(Root(), "golden", "known_divergences.yaml"))
	if err != nil {
		return nil, err
	}

	var out []Divergence

	for _, r := range rows {
		d := Divergence{Method: r["method"], Variant: "*", Seed: "*", Reason: r["reason"], Category: r["category"]}
		if v, ok := r["variant"]; ok {
			d.Variant = v
		}

		if v, ok := r["seed"]; ok {
			d.Seed = v
		}

		for k := range r {
			switch k {
			case "method", "variant", "seed", "reason", "category":
			default:
				return nil, fmt.Errorf("known_divergences.yaml: unknown key %q", k)
			}
		}

		if d.Method == "" || d.Reason == "" || d.Category == "" {
			return nil, fmt.Errorf("known_divergences.yaml: entry %v needs method, reason and category", r)
		}

		out = append(out, d)
	}

	return out, nil
}

// Skipped is one entry of golden/skipped.yaml: an API method deliberately not exercised.
type Skipped struct{ Method, Reason string }

// LoadSkipped parses golden/skipped.yaml.
func LoadSkipped() ([]Skipped, error) {
	rows, err := loadYAMLList(filepath.Join(Root(), "golden", "skipped.yaml"))
	if err != nil {
		return nil, err
	}

	var out []Skipped

	for _, r := range rows {
		if r["method"] == "" || r["reason"] == "" {
			return nil, fmt.Errorf("skipped.yaml: entry %v needs method and reason", r)
		}

		out = append(out, Skipped{r["method"], r["reason"]})
	}

	return out, nil
}

// Matches reports whether the entry covers a per-method case ID "pm:<method>#<variant>@<seed>".
func (d Divergence) Matches(c *Case) bool {
	if d.Method != c.Method() {
		return false
	}

	id := c.ID
	at := strings.LastIndex(id, "@")

	hash := strings.LastIndex(id, "#")
	if at < 0 || hash < 0 || hash > at {
		return d.Variant == "*" && d.Seed == "*"
	}

	variant, seed := id[hash+1:at], id[at+1:]

	return (d.Variant == "*" || d.Variant == variant) && (d.Seed == "*" || d.Seed == seed)
}

// Tally accumulates dispositions for reporting.
type Tally struct {
	Exact, Normalized, Divergent int
}

func (t *Tally) add(d Disposition) {
	switch d {
	case Exact:
		t.Exact++
	case Normalized:
		t.Normalized++
	default:
		t.Divergent++
	}
}

// Total is the number of cases tallied.
func (t Tally) Total() int { return t.Exact + t.Normalized + t.Divergent }

// PassRate is (exact+normalized)/total.
func (t Tally) PassRate() float64 {
	if t.Total() == 0 {
		return 0
	}

	return float64(t.Exact+t.Normalized) / float64(t.Total())
}

// Report is written to reports/conformance-<engine>.json.
type Report struct {
	Engine     string            `json:"engine"`
	ByCategory map[string]*Tally `json:"byCategory"`
	ByModule   map[string]*Tally `json:"byModule"`
	// Divergent lists, per category, the first divergence of every divergent case.
	Divergent map[string][]string `json:"divergent,omitempty"`
}

// NewReport returns an empty report.
func NewReport(engine string) *Report {
	return &Report{Engine: engine, ByCategory: map[string]*Tally{}, ByModule: map[string]*Tally{}, Divergent: map[string][]string{}}
}

// Add records one outcome under its category and (for per-method cases) its module.
func (r *Report) Add(o Outcome) {
	cat := o.Case.Kind
	if r.ByCategory[cat] == nil {
		r.ByCategory[cat] = &Tally{}
	}

	r.ByCategory[cat].add(o.Disposition)

	if o.Case.Kind == "per-method" {
		m := o.Case.Module()
		if r.ByModule[m] == nil {
			r.ByModule[m] = &Tally{}
		}

		r.ByModule[m].add(o.Disposition)
	}

	if o.Disposition == Divergent {
		r.Divergent[cat] = append(r.Divergent[cat], fmt.Sprintf("%s: %s", o.Case.ID, o.describe()))
	}
}

func (o Outcome) describe() string {
	return fmt.Sprintf("call #%d expected %s got %s", o.Index, o.Expected, o.Got)
}

// Write stores the report as JSON under reports/, merging with an existing file for the same engine so
// separate tests can contribute their categories.
func (r *Report) Write() error {
	path := filepath.Join(ReportsDir(), "conformance-"+r.Engine+".json")

	merged := NewReport(r.Engine)
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, merged)
		if merged.ByCategory == nil {
			merged.ByCategory = map[string]*Tally{}
		}

		if merged.ByModule == nil {
			merged.ByModule = map[string]*Tally{}
		}
	}

	maps.Copy(merged.ByCategory, r.ByCategory)

	maps.Copy(merged.ByModule, r.ByModule)

	if merged.Divergent == nil {
		merged.Divergent = map[string][]string{}
	}

	for k := range r.ByCategory {
		delete(merged.Divergent, k)
	}

	for k, v := range r.Divergent {
		sort.Strings(v)
		merged.Divergent[k] = v
	}

	b, err := json.MarshalIndent(merged, "", " ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0o644)
}
