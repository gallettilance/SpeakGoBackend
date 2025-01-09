package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Exercise struct {
	ID				primitive.ObjectID	`bson:"_id"`
	Name			*string				`json:"name" validate:"required,min=2,max=100"`
	Description		*string				`json:"description" validate:"required"`
	VideoURL 		string				`json:"video_url" bson:"video_url"`
	Tags 			[]string			`json:"tags"`
	CreatedAt		time.Time			`json:"created_at" bson:"created_at"`
	UpdatedAt		time.Time			`json:"updated_at" bson:"updated_at"`
	ExerciseID		string				`json:"exercise_id" bson:"exercise_id"`
	IsGlobal    	bool               	`json:"is_global" bson:"is_global"`
	TherapistID 	string             	`json:"therapist_id" bson:"therapist_id"`
}