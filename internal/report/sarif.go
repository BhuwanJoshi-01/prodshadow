package report

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
)

// SARIF 2.1.0 output, so findings appear as code scanning alerts and pull
// request annotations on the migration lines that caused them.

const (
	sarifSchema  = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion = "2.1.0"
	toolURI      = "https://github.com/BhuwanJoshi-01/Open-Source-1"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	ShortDescription sarifText         `json:"shortDescription"`
	HelpURI          string            `json:"helpUri,omitempty"`
	Default          sarifRuleDefaults `json:"defaultConfiguration"`
}

type sarifRuleDefaults struct {
	Level string `json:"level"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID       string             `json:"ruleId"`
	Level        string             `json:"level"`
	Message      sarifText          `json:"message"`
	Locations    []sarifLocation    `json:"locations"`
	Suppressions []sarifSuppression `json:"suppressions,omitempty"`
	Properties   map[string]any     `json:"properties,omitempty"`
}

type sarifLocation struct {
	Physical sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	Artifact sarifArtifact `json:"artifactLocation"`
	Region   sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

type sarifSuppression struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification"`
}

// sarifLevel maps severities to SARIF levels. Inconclusive results are
// warnings: they need attention but are not proven problems.
func sarifLevel(s evidence.Severity) string {
	switch s {
	case evidence.Critical:
		return "error"
	case evidence.Warning, evidence.Inconclusive:
		return "warning"
	default:
		return "note"
	}
}

// RulePageURL returns the address of a rule's documentation page, which
// explains the hazard, shows an example and links the PostgreSQL manual.
func RulePageURL(id string) string {
	return toolURI + "/blob/main/docs/rules/" + id + ".md"
}

// ruleCatalog describes every known rule for the SARIF driver.
func ruleCatalog() map[string]sarifRule {
	out := map[string]sarifRule{}
	for _, r := range sqlinspect.Rules() {
		out[r.ID] = sarifRule{ID: r.ID, Name: r.Name, ShortDescription: sarifText{r.Summary}, HelpURI: RulePageURL(r.ID),
			Default: sarifRuleDefaults{sarifLevel(r.Severity)}}
	}
	for _, r := range risk.ObservedRules {
		out[r.ID] = sarifRule{ID: r.ID, Name: r.Name, ShortDescription: sarifText{r.Summary},
			HelpURI: RulePageURL(r.ID), Default: sarifRuleDefaults{sarifLevel(r.Severity)}}
	}
	return out
}

// SARIF renders findings as a SARIF log. Findings that are not tied to a
// line (for example a scenario's latency) are anchored to fallbackFile, the
// first migration file, because code scanning requires a location.
func SARIF(findings []risk.Finding, toolVersion, fallbackFile string) ([]byte, error) {
	catalog := ruleCatalog()
	used := map[string]bool{}
	results := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		file, line := f.File, f.Line
		if file == "" {
			file, line = fallbackFile, 1
		}
		if file == "" {
			continue
		}
		if line < 1 {
			line = 1
		}
		res := sarifResult{
			RuleID:  f.RuleID,
			Level:   sarifLevel(f.Severity),
			Message: sarifText{f.Message},
			Locations: []sarifLocation{{Physical: sarifPhysical{
				Artifact: sarifArtifact{URI: file}, Region: sarifRegion{StartLine: line},
			}}},
			Properties: map[string]any{"severity": f.Severity, "confidence": f.Confidence, "evidenceKind": f.Evidence},
		}
		if f.Scenario != "" {
			res.Properties["scenario"] = f.Scenario
		}
		if f.Suppressed {
			res.Suppressions = []sarifSuppression{{Kind: "inSource", Justification: f.SuppressionReason}}
		}
		used[f.RuleID] = true
		results = append(results, res)
	}
	rules := make([]sarifRule, 0, len(used))
	for id := range used {
		r, ok := catalog[id]
		if !ok {
			r = sarifRule{ID: id, Name: id, ShortDescription: sarifText{id}, Default: sarifRuleDefaults{"warning"}}
		}
		rules = append(rules, r)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })

	log := sarifLog{Schema: sarifSchema, Version: sarifVersion, Runs: []sarifRun{{
		Tool:    sarifTool{Driver: sarifDriver{Name: "ProdShadow", Version: toolVersion, InformationURI: toolURI, Rules: rules}},
		Results: results,
	}}}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(log); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SARIF renders the report's findings.
func (r *Report) SARIF() ([]byte, error) {
	fallback := ""
	if len(r.Migration.Statements) > 0 {
		fallback = r.Migration.Statements[0].File
	}
	return SARIF(r.Findings, r.Tool.Version, fallback)
}
