package config

// Default values. They are deliberately conservative: a short experiment that
// finishes in CI-friendly time on a small fixture.
const (
	DefaultPostgresVersion       = "17"
	DefaultDatabase              = "app_test"
	DefaultProvider              = "docker"
	DefaultNetwork               = "none"
	DefaultMemoryMB              = 2048
	DefaultCPUs                  = 2
	DefaultEphemeralDiskMB       = 4096
	DefaultPidsLimit             = 512
	DefaultStartupTimeoutSeconds = 120
	DefaultRepresentation        = "manual-fixture"
	DefaultMaxRows               = 50_000_000
	DefaultWarmupSeconds         = 10
	DefaultMeasurementSeconds    = 45
	DefaultConcurrency           = 16
	DefaultRepeats               = 3
	DefaultSeed                  = 1
	DefaultMinSamples            = 200
	DefaultLockTimeoutMS         = 1500
	DefaultRetryBackoffMS        = 500
	DefaultStatementTimeoutMS    = 60_000
	DefaultStartAfterSeconds     = 5
	DefaultRunAs                 = "owner"
	DefaultTransaction           = "auto"
	DefaultInconclusive          = "fail"
	DefaultReportDirectory       = "./prodshadow-results"
	DefaultSampleIntervalMS      = 100
)

// Default policy thresholds; each can be overridden individually.
var (
	DefaultMaxErrorRateIncreasePctPoints = 1.0
	DefaultMaxP95LatencyRatio            = 1.5
	DefaultMaxLockWaitMS                 = 2000.0
)

// DefaultImage returns the image used when postgres.image is unset.
func DefaultImage(major string) string {
	return "postgres:" + major + "-alpine"
}

// ApplyDefaults fills zero-valued fields. It never overrides explicit values.
func (c *Config) ApplyDefaults() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}

	pg := &c.Postgres
	if pg.Version == "" {
		pg.Version = DefaultPostgresVersion
	}
	if pg.Image == "" {
		pg.Image = DefaultImage(pg.Version)
	}
	if pg.Database == "" {
		pg.Database = DefaultDatabase
	}

	iso := &pg.Isolation
	if iso.Provider == "" {
		iso.Provider = DefaultProvider
	}
	if iso.Network == "" {
		iso.Network = DefaultNetwork
	}
	if iso.MemoryMB == 0 {
		iso.MemoryMB = DefaultMemoryMB
	}
	if iso.CPUs == 0 {
		iso.CPUs = DefaultCPUs
	}
	if iso.EphemeralDiskMB == 0 {
		iso.EphemeralDiskMB = DefaultEphemeralDiskMB
	}
	if iso.PidsLimit == 0 {
		iso.PidsLimit = DefaultPidsLimit
	}
	if iso.StartupTimeoutSeconds == 0 {
		iso.StartupTimeoutSeconds = DefaultStartupTimeoutSeconds
	}

	if c.Fixture.Representation == "" {
		c.Fixture.Representation = DefaultRepresentation
	}
	if c.Fixture.MaxRows == 0 {
		c.Fixture.MaxRows = DefaultMaxRows
	}

	w := &c.Workload
	if w.MeasurementSeconds == 0 {
		w.MeasurementSeconds = DefaultMeasurementSeconds
	}
	if w.Concurrency == 0 {
		w.Concurrency = DefaultConcurrency
	}
	if w.Repeats == 0 {
		w.Repeats = DefaultRepeats
	}
	if w.Seed == 0 {
		w.Seed = DefaultSeed
	}
	if w.MinSamplesPerScenario == 0 {
		w.MinSamplesPerScenario = DefaultMinSamples
	}

	m := &c.Migration
	if m.LockTimeoutMS == 0 {
		m.LockTimeoutMS = DefaultLockTimeoutMS
	}
	if m.StatementTimeoutMS == 0 {
		m.StatementTimeoutMS = DefaultStatementTimeoutMS
	}
	if m.RunAs == "" {
		m.RunAs = DefaultRunAs
	}
	if m.RetryBackoffMS == 0 {
		m.RetryBackoffMS = DefaultRetryBackoffMS
	}
	if m.Transaction == "" {
		m.Transaction = DefaultTransaction
	}

	p := &c.Policy
	if len(p.FailOn) == 0 {
		p.FailOn = []string{"critical"}
	}
	if p.Inconclusive == "" {
		p.Inconclusive = DefaultInconclusive
	}
	if p.MaxErrorRateIncreasePctPoints == nil {
		v := DefaultMaxErrorRateIncreasePctPoints
		p.MaxErrorRateIncreasePctPoints = &v
	}
	if p.MaxP95LatencyRatio == nil {
		v := DefaultMaxP95LatencyRatio
		p.MaxP95LatencyRatio = &v
	}
	if p.MaxLockWaitMS == nil {
		v := DefaultMaxLockWaitMS
		p.MaxLockWaitMS = &v
	}

	if c.Report.Directory == "" {
		c.Report.Directory = DefaultReportDirectory
	}
	if len(c.Report.Formats) == 0 {
		c.Report.Formats = []string{"json", "markdown"}
	}

	if c.Telemetry.SampleIntervalMS == 0 {
		c.Telemetry.SampleIntervalMS = DefaultSampleIntervalMS
	}
}
