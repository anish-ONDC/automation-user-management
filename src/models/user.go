package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User is one Workbench user. GitHub users fill Login/AvatarURL; ONDC users fill
// the ONDC fields. The Mongo _id is the user_id every other feature keys on
// (notes, comments, scenario preferences, MCP keys), so it never changes.
type User struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Login     string             `json:"login" bson:"login"`
	Name      string             `json:"name" bson:"name"`
	AvatarURL string             `json:"avatar_url" bson:"avatar_url"`
	Email     string             `json:"email" bson:"email"`

	AuthProvider  string             `json:"auth_provider,omitempty" bson:"auth_provider,omitempty"`
	ONDCUserID    int64              `json:"ondc_user_id,omitempty" bson:"ondc_user_id,omitempty"`
	FirstName     string             `json:"first_name,omitempty" bson:"first_name,omitempty"`
	LastName      string             `json:"last_name,omitempty" bson:"last_name,omitempty"`
	ONDCStatus    int                `json:"ondc_status,omitempty" bson:"ondc_status,omitempty"`
	Organisations []UserOrganisation `json:"organisations,omitempty" bson:"organisations,omitempty"`
	LastLoginAt   time.Time          `json:"last_login_at,omitempty" bson:"last_login_at,omitempty"`
}

type UserOrganisation struct {
	OrganisationID   string   `json:"organisation_id" bson:"organisation_id"`
	OrganisationName string   `json:"organisation_name" bson:"organisation_name"`
	NetworkStatus    string   `json:"network_status" bson:"network_status"`
	ParticipantID    string   `json:"participant_id,omitempty" bson:"participant_id,omitempty"`
	NPType           string   `json:"np_type" bson:"np_type"`
	IsRepresentative bool     `json:"is_representative" bson:"is_representative"`
	Status           int      `json:"status" bson:"status"`
	SubscriberIDs    []string `json:"subscriber_ids" bson:"subscriber_ids"`
}
