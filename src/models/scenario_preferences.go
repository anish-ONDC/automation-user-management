package models

type PreferenceConfig struct {
	SubscriberURL string `json:"subscriber_url" bson:"subscriber_url" validate:"required"`
	Domain        string `json:"domain"         bson:"domain"`
	Version       string `json:"version"        bson:"version"`
	NpType        string `json:"np_type"        bson:"np_type"`
	Env           string `json:"env"            bson:"env"`
	UsecaseID     string `json:"usecase_id"     bson:"usecase_id"`
	OrgID         string `json:"org_id,omitempty" bson:"org_id,omitempty"`
}

// PreferenceEntry stores a single preference keyed by "k" to avoid BSON
// dot-in-field-name restrictions (e.g. version "2.0.0" in a map key).
type PreferenceEntry struct {
	ConfigKey        string `json:"-"  bson:"k"`
	PreferenceConfig        `bson:",inline"`
}

type UserScenarioPreferences struct {
	UserID      string            `json:"-"           bson:"user_id"`
	Preferences []PreferenceEntry `json:"preferences" bson:"preferences"`
}
