package handlers

import (
	"testing"

	"automation-developer-guide/src/models"
)

func TestPickOrganisation(t *testing.T) {
	orgs := []models.UserOrganisation{
		{OrganisationID: "11899", OrganisationName: "First"},
		{OrganisationID: "22000", OrganisationName: "Second"},
	}

	if got := pickOrganisation(nil, "11899"); got != nil {
		t.Fatalf("no organisations: got %+v, want nil", got)
	}
	if got := pickOrganisation(orgs, ""); got.OrganisationID != "11899" {
		t.Fatalf("no choice: got %s, want the first", got.OrganisationID)
	}
	if got := pickOrganisation(orgs, "22000"); got.OrganisationID != "22000" {
		t.Fatalf("saved choice: got %s, want 22000", got.OrganisationID)
	}
	// The user may have left the chosen organisation since the last login.
	if got := pickOrganisation(orgs, "99999"); got.OrganisationID != "11899" {
		t.Fatalf("stale choice: got %s, want the first", got.OrganisationID)
	}
}
