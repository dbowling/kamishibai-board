package seed

import "github.com/dbowling/kamishibai/backend/internal/domain"

// The demo dataset.
//
// Everything is addressed by a natural key (user email, team name, board name
// within a team, card title within a board) so that seeding can find and update
// what it created rather than inserting duplicates. That is what makes `mise run
// seed` safe to run repeatedly.
//
// All email addresses use the reserved .test TLD so seeded accounts can never
// collide with, or be mistaken for, real ones.

type userSpec struct {
	Email string
	Name  string
	Role  string
}

type teamSpec struct {
	Name        string
	Description string
	MemberEmail []string
	SortOrder   int
}

type boardSpec struct {
	Team        string
	Name        string
	Description string
	SortOrder   int
}

type linkSpec struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type checklistSpec struct {
	Text string `json:"text"`
}

type cardSpec struct {
	Board        string
	Title        string
	Summary      string
	Cadence      domain.Cadence
	Instructions string
	Links        []linkSpec
	Checklist    []checklistSpec
	SortOrder    int

	// Reliability drives the generated history: the share of past periods that
	// were completed. It exists so the seeded reports show a realistic spread
	// rather than a flat 100%.
	Reliability float64
}

const (
	teamPlatform = "Platform Engineering"
	teamSecurity = "Security"

	boardPlatformTriage = "Daily Triage"
	boardPlatformCap    = "Capacity & Cost"
	boardSecurityTriage = "Security Triage"
)

func demoUsers() []userSpec {
	return []userSpec{
		{Email: "admin@example.test", Name: "Ada Admin", Role: "admin"},
		{Email: "dana@example.test", Name: "Dana Ops", Role: "user"},
		{Email: "raj@example.test", Name: "Raj Patel", Role: "user"},
		{Email: "mei@example.test", Name: "Mei Chen", Role: "user"},
		{Email: "sam@example.test", Name: "Sam Okafor", Role: "user"},
	}
}

func demoTeams() []teamSpec {
	return []teamSpec{
		{
			Name:        teamPlatform,
			Description: "Owns the clusters, pipelines and shared infrastructure.",
			// Dana and Raj are on both teams, which exercises multi-team membership.
			MemberEmail: []string{"dana@example.test", "raj@example.test", "mei@example.test"},
			SortOrder:   1,
		},
		{
			Name:        teamSecurity,
			Description: "Handles vulnerability response and access reviews.",
			MemberEmail: []string{"dana@example.test", "raj@example.test", "sam@example.test"},
			SortOrder:   2,
		},
	}
}

func demoBoards() []boardSpec {
	return []boardSpec{
		{Team: teamPlatform, Name: boardPlatformTriage, Description: "Recurring health checks for production infrastructure.", SortOrder: 1},
		{Team: teamPlatform, Name: boardPlatformCap, Description: "Longer-cycle capacity, cost and resilience work.", SortOrder: 2},
		{Team: teamSecurity, Name: boardSecurityTriage, Description: "Recurring security hygiene and review tasks.", SortOrder: 1},
	}
}

func demoCards() []cardSpec {
	return []cardSpec{
		// The card from the original brief: a Grafana link plus troubleshooting
		// steps.
		{
			Board:   boardPlatformTriage,
			Title:   "Verify Backups",
			Summary: "Confirm last night's backups completed for every production database.",
			Cadence: domain.Daily,
			Instructions: "<p>Check that every production database completed its nightly backup, " +
				"and that the reported size is in line with the previous run. " +
				"A backup that succeeds but shrinks sharply is still a failure.</p>" +
				"<p>If anything failed, work the troubleshooting steps below before escalating.</p>",
			Links: []linkSpec{
				{Label: "Grafana: backup success/failure", URL: "https://grafana.example.test/d/backups/backup-overview"},
				{Label: "Runbook: backup failures", URL: "https://wiki.example.test/runbooks/backup-failure"},
			},
			Checklist: []checklistSpec{
				{Text: "Check the backup job logs for the failing database"},
				{Text: "Confirm the storage target has free capacity and valid credentials"},
				{Text: "Re-run the backup manually and, if it fails again, page the on-call DBA"},
			},
			SortOrder:   1,
			Reliability: 0.94,
		},
		{
			Board:   boardPlatformTriage,
			Title:   "Triage Overnight Alerts",
			Summary: "Review and close out every alert that fired outside working hours.",
			Cadence: domain.Daily,
			Instructions: "<p>Work through the overnight alert queue. Every alert should end up " +
				"either resolved, converted into a ticket, or silenced with a documented reason.</p>",
			Links: []linkSpec{
				{Label: "Alertmanager queue", URL: "https://alerts.example.test/#/alerts"},
			},
			Checklist: []checklistSpec{
				{Text: "Group related alerts so one incident is not triaged five times"},
				{Text: "Raise a ticket for anything needing follow-up work"},
				{Text: "Note any alert that fired without being actionable, for tuning"},
			},
			SortOrder:   2,
			Reliability: 0.88,
		},
		{
			Board:   boardPlatformTriage,
			Title:   "Check Certificate Expiry",
			Summary: "Confirm no TLS certificate expires within 30 days.",
			Cadence: domain.Daily,
			Instructions: "<p>Review the certificate expiry dashboard. Anything inside 30 days needs " +
				"a renewal ticket; anything inside 7 days needs renewing today.</p>",
			Links: []linkSpec{
				{Label: "Grafana: certificate expiry", URL: "https://grafana.example.test/d/certs/tls-expiry"},
			},
			SortOrder:   3,
			Reliability: 0.72,
		},
		{
			Board:   boardPlatformTriage,
			Title:   "Review Failed Pipelines",
			Summary: "Investigate CI pipelines that failed during the week.",
			Cadence: domain.Weekly,
			Instructions: "<p>Look for repeat failures and flaky tests rather than one-off breakages. " +
				"A test that fails one run in ten is a bug waiting to hide a real regression.</p>",
			Links: []linkSpec{
				{Label: "CI dashboard", URL: "https://ci.example.test/pipelines?status=failed"},
			},
			Checklist: []checklistSpec{
				{Text: "Identify the three most frequently failing jobs"},
				{Text: "Open or update a ticket for each flaky test"},
			},
			SortOrder:   4,
			Reliability: 0.81,
		},
		{
			Board:        boardPlatformTriage,
			Title:        "Patch Review",
			Summary:      "Review and schedule outstanding OS and dependency patches.",
			Cadence:      domain.Weekly,
			Instructions: "<p>Check which hosts and images are behind on patches and schedule the rollout.</p>",
			SortOrder:    5,
			Reliability:  0.66,
		},

		{
			Board:   boardPlatformCap,
			Title:   "Capacity Forecast Review",
			Summary: "Compare actual growth against the forecast and adjust headroom.",
			Cadence: domain.Monthly,
			Instructions: "<p>Review cluster CPU, memory and storage growth for the month. " +
				"Flag anything projected to exhaust headroom within two quarters.</p>",
			Links: []linkSpec{
				{Label: "Grafana: cluster capacity", URL: "https://grafana.example.test/d/capacity/cluster-capacity"},
			},
			SortOrder:   1,
			Reliability: 0.75,
		},
		{
			Board:        boardPlatformCap,
			Title:        "Cloud Cost Review",
			Summary:      "Reconcile the monthly cloud bill against budget and tag coverage.",
			Cadence:      domain.Monthly,
			Instructions: "<p>Break the bill down by team and flag any line item that moved more than 15% month on month.</p>",
			SortOrder:    2,
			Reliability:  0.83,
		},
		{
			Board:   boardPlatformCap,
			Title:   "Disaster Recovery Drill",
			Summary: "Restore production from backup into an isolated environment.",
			Cadence: domain.Quarterly,
			Instructions: "<p>A backup is only real once it has been restored. Restore the most recent " +
				"production backup into the isolated recovery environment and verify the application starts " +
				"and serves traffic.</p><p>Record the wall-clock time to restore; that number is the real RTO.</p>",
			Links: []linkSpec{
				{Label: "Runbook: DR drill", URL: "https://wiki.example.test/runbooks/dr-drill"},
			},
			Checklist: []checklistSpec{
				{Text: "Restore the latest backup into the recovery environment"},
				{Text: "Verify the application starts and passes smoke tests"},
				{Text: "Record the time to restore and compare it to the documented RTO"},
			},
			SortOrder:   3,
			Reliability: 0.6,
		},
		{
			Board:        boardPlatformCap,
			Title:        "Architecture Review",
			Summary:      "Annual review of the platform architecture and its documented decisions.",
			Cadence:      domain.Annual,
			Instructions: "<p>Revisit the architecture decision records and retire any that no longer reflect reality.</p>",
			SortOrder:    4,
			Reliability:  0.5,
		},

		{
			Board:   boardSecurityTriage,
			Title:   "Review New CVEs",
			Summary: "Assess newly published CVEs against the deployed software inventory.",
			Cadence: domain.Daily,
			Instructions: "<p>Compare the day's CVE feed against the software inventory and " +
				"score anything that matches. Critical findings in internet-facing services are same-day work.</p>",
			Links: []linkSpec{
				{Label: "CVE feed", URL: "https://cve.example.test/recent"},
				{Label: "Software inventory", URL: "https://inventory.example.test"},
			},
			SortOrder:   1,
			Reliability: 0.9,
		},
		{
			Board:   boardSecurityTriage,
			Title:   "Access Review",
			Summary: "Confirm production access still matches current roles.",
			Cadence: domain.Monthly,
			Instructions: "<p>Reconcile production access groups against the current team roster. " +
				"Anybody who changed team or left should no longer appear.</p>",
			Checklist: []checklistSpec{
				{Text: "Export the current production access groups"},
				{Text: "Compare against the HR roster"},
				{Text: "Revoke anything unexpected and record why"},
			},
			SortOrder:   2,
			Reliability: 0.79,
		},
		{
			Board:        boardSecurityTriage,
			Title:        "Penetration Test Follow-up",
			Summary:      "Review the status of findings from the last penetration test.",
			Cadence:      domain.Quarterly,
			Instructions: "<p>Walk the open findings list and confirm each has an owner and a target date.</p>",
			SortOrder:    3,
			Reliability:  0.55,
		},
	}
}
