package report

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

// JUnit XML lets CI systems that understand test reports (GitLab, Jenkins,
// Buildkite, ...) show findings next to unit test results. Each finding is
// a test case: blocking findings fail, suppressed findings are skipped,
// everything else passes with its message as output.

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Time     float64      `xml:"time,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Time     float64     `xml:"time,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	ClassName string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// JUnit renders the report as JUnit XML.
func (r *Report) JUnit() ([]byte, error) {
	suite := junitSuite{Name: "prodshadow/" + r.Project, Time: r.DurationSec}
	failsCI := func(id string) bool {
		if slices.Contains(r.Decision.FailedRules, id) {
			return true
		}
		return r.Decision.ExitCode != 0 && slices.Contains(r.Decision.Inconclusive, id)
	}

	// A first case carries the overall decision, so a run with errors and
	// no findings still fails.
	decision := junitCase{ClassName: "prodshadow.decision", Name: "decision: " + r.Decision.Outcome, SystemOut: r.Decision.Message}
	if r.Decision.ExitCode != 0 {
		decision.Failure = &junitFailure{Message: r.Decision.Message, Type: r.Decision.Outcome, Text: strings.Join(r.Errors, "\n")}
	}
	suite.Cases = append(suite.Cases, decision)

	for _, f := range r.Findings {
		c := junitCase{
			ClassName: "prodshadow." + string(f.Evidence),
			Name:      fmt.Sprintf("%s %s (%s)", f.RuleID, f.Rule, f.Where("–")),
		}
		detail := f.Message
		if f.Remediation != "" {
			detail += "\nRemediation: " + f.Remediation
		}
		switch {
		case f.Suppressed:
			c.Skipped = &junitSkipped{Message: "suppressed: " + f.SuppressionReason}
		case failsCI(f.RuleID):
			c.Failure = &junitFailure{Message: f.Message, Type: string(f.Severity), Text: detail}
		default:
			c.SystemOut = fmt.Sprintf("[%s, confidence %s] %s", f.Severity, f.Confidence, detail)
		}
		suite.Cases = append(suite.Cases, c)
	}
	for _, c := range suite.Cases {
		suite.Tests++
		if c.Failure != nil {
			suite.Failures++
		}
		if c.Skipped != nil {
			suite.Skipped++
		}
	}
	doc := junitSuites{Name: "ProdShadow", Tests: suite.Tests, Failures: suite.Failures, Skipped: suite.Skipped,
		Time: suite.Time, Suites: []junitSuite{suite}}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
