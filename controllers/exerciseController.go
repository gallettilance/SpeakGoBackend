package controllers

import (
	"context"
	"fmt"
	"golang-speakbackend/database"
	"golang-speakbackend/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var exerciseCollection *mongo.Collection = database.OpenCollection(database.Client, "exercise")
var validate = validator.New()

// GetGlobalExercises retrieves all global exercises with pagination
func GetGlobalExercises() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Pagination parameters
		recordPerPage, err := strconv.Atoi(c.Query("recordPerPage"))
		if err != nil || recordPerPage < 1 {
			recordPerPage = 10
		}

		page, err := strconv.Atoi(c.Query("page"))
		if err != nil || page < 1 {
			page = 1
		}

		startIndex := (page - 1) * recordPerPage

		// Match stage for global exercises
		matchStage := bson.D{{Key: "$match", Value: bson.D{{Key: "is_global", Value: true}}}}

		// Count total number of global exercises
		countStage := bson.D{{Key: "$count", Value: "total"}}

		// Skip and Limit for pagination
		skipStage := bson.D{{Key: "$skip", Value: startIndex}}
		limitStage := bson.D{{Key: "$limit", Value: recordPerPage}}

		// Project stage to include necessary fields
		projectStage := bson.D{
			{Key: "$project", Value: bson.D{
				{Key: "_id", Value: 1},
				{Key: "name", Value: 1},
				{Key: "description", Value: 1},
				{Key: "video_url", Value: 1},
				{Key: "tags", Value: 1},
				{Key: "created_at", Value: 1},
				{Key: "updated_at", Value: 1},
				{Key: "exercise_id", Value: 1},
				{Key: "is_global", Value: 1},
				{Key: "therapist_id", Value: 1},
			}},
		}

		// Aggregate total global exercises count
		countResult, err := exerciseCollection.Aggregate(ctx, mongo.Pipeline{matchStage, countStage})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while counting global exercises"})
			return
		}
		var countData []bson.M
		if err = countResult.All(ctx, &countData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while counting global exercises"})
			return
		}
		totalCount := 0
		if len(countData) > 0 {
			if tc, ok := countData[0]["total"].(int32); ok {
				totalCount = int(tc)
			} else if tc, ok := countData[0]["total"].(int64); ok {
				totalCount = int(tc)
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid count type"})
				return
			}
		}

		// Aggregate global exercises with pagination
		result, err := exerciseCollection.Aggregate(ctx, mongo.Pipeline{matchStage, skipStage, limitStage, projectStage})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching global exercises"})
			return
		}

		var exercises []bson.M
		if err = result.All(ctx, &exercises); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching global exercises"})
			return
		}

		response := gin.H{
			"total":     totalCount,
			"exercises": exercises,
		}

		c.JSON(http.StatusOK, response)
	}
}

// GetTherapistExercises retrieves exercises created by a specific therapist and all global exercises
func GetTherapistExercises() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Retrieve therapist_id from query parameters
		therapistID := c.Query("therapist_id")
		if therapistID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "therapist_id is required"})
			return
		}

		// Pagination parameters
		recordPerPage, err := strconv.Atoi(c.Query("recordPerPage"))
		if err != nil || recordPerPage < 1 {
			recordPerPage = 10
		}

		page, err := strconv.Atoi(c.Query("page"))
		if err != nil || page < 1 {
			page = 1
		}

		startIndex := (page - 1) * recordPerPage

		// Match stage for therapist-specific and global exercises
		matchStage := bson.D{{
			Key: "$match", Value: bson.D{
				{Key: "$or", Value: bson.A{
					bson.D{{Key: "therapist_id", Value: therapistID}},
					bson.D{{Key: "is_global", Value: true}},
				}},
			},
		}}

		// Count total number of relevant exercises
		countStage := bson.D{{Key: "$count", Value: "total"}}

		// Skip and Limit for pagination
		skipStage := bson.D{{Key: "$skip", Value: startIndex}}
		limitStage := bson.D{{Key: "$limit", Value: recordPerPage}}

		// Project stage to include necessary fields
		projectStage := bson.D{
			{Key: "$project", Value: bson.D{
				{Key: "_id", Value: 1},
				{Key: "name", Value: 1},
				{Key: "description", Value: 1},
				{Key: "video_url", Value: 1},
				{Key: "tags", Value: 1},
				{Key: "created_at", Value: 1},
				{Key: "updated_at", Value: 1},
				{Key: "exercise_id", Value: 1},
				{Key: "therapist_id", Value: 1},
				{Key: "is_global", Value: 1},
			}},
		}

		// Aggregate total relevant exercises count
		countResult, err := exerciseCollection.Aggregate(ctx, mongo.Pipeline{matchStage, countStage})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while counting exercises"})
			return
		}
		var countData []bson.M
		if err = countResult.All(ctx, &countData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while counting exercises"})
			return
		}
		totalCount := 0
		if len(countData) > 0 {
			if tc, ok := countData[0]["total"].(int32); ok {
				totalCount = int(tc)
			} else if tc, ok := countData[0]["total"].(int64); ok {
				totalCount = int(tc)
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid count type"})
				return
			}
		}

		// Aggregate exercises with pagination
		result, err := exerciseCollection.Aggregate(ctx, mongo.Pipeline{matchStage, skipStage, limitStage, projectStage})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching exercises"})
			return
		}

		var exercises []bson.M
		if err = result.All(ctx, &exercises); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching exercises"})
			return
		}

		response := gin.H{
			"total":     totalCount,
			"exercises": exercises,
		}

		c.JSON(http.StatusOK, response)
	}
}

// GetExercise retrieves a single exercise by its ID
func GetExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		exerciseID := c.Param("exercise_id")
		var exercise models.Exercise

		err := exerciseCollection.FindOne(ctx, bson.M{"exercise_id": exerciseID}).Decode(&exercise)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching exercise"})
			return
		}
		c.JSON(http.StatusOK, exercise)
	}
}

// CreateExercise creates a new exercise
func CreateExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var exercise models.Exercise

		if err := c.BindJSON(&exercise); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate the exercise struct
		validationError := validate.Struct(exercise)
		if validationError != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationError.Error()})
			return
		}

		exercise.CreatedAt, _ = time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
		exercise.UpdatedAt, _ = time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
		exercise.ID = primitive.NewObjectID()
		exercise.ExerciseID = exercise.ID.Hex()

		// Insert the exercise into the database
		result, insertErr := exerciseCollection.InsertOne(ctx, exercise)
		if insertErr != nil {
			msg := fmt.Sprintf("Error while inserting exercise: %s", insertErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// UpdateExercise updates an existing exercise by its ID
func UpdateExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		exerciseID := c.Param("exercise_id")
		var exercise models.Exercise

		if err := c.BindJSON(&exercise); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		filter := bson.M{"exercise_id": exerciseID}
		var updateObj bson.D

		// Prepare update fields
		if exercise.Name != nil {
			updateObj = append(updateObj, bson.E{Key: "name", Value: exercise.Name})
		}
		if exercise.Description != nil {
			updateObj = append(updateObj, bson.E{Key: "description", Value: exercise.Description})
		}
		if exercise.VideoURL != "" {
			updateObj = append(updateObj, bson.E{Key: "video_url", Value: exercise.VideoURL})
		}
		if len(exercise.Tags) > 0 {
			updateObj = append(updateObj, bson.E{Key: "tags", Value: exercise.Tags})
		}
		if exercise.IsGlobal {
			updateObj = append(updateObj, bson.E{Key: "is_global", Value: exercise.IsGlobal})
		}
		if exercise.TherapistID != "" {
			updateObj = append(updateObj, bson.E{Key: "therapist_id", Value: exercise.TherapistID})
		}

		// Update the updated_at timestamp
		exercise.UpdatedAt = time.Now().UTC()
		updateObj = append(updateObj, bson.E{Key: "updated_at", Value: exercise.UpdatedAt})

		upsert := true
		opt := options.UpdateOptions{
			Upsert: &upsert,
		}

		// Perform the update
		result, err := exerciseCollection.UpdateOne(
			ctx,
			filter,
			bson.D{
				{Key: "$set", Value: updateObj},
			},
			&opt,
		)

		if err != nil {
			msg := fmt.Sprintf("Exercise update failed: %s", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// DeleteExercise deletes an exercise by its ID
func DeleteExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		exerciseID := c.Param("exercise_id")
		filter := bson.M{"exercise_id": exerciseID}

		result, err := exerciseCollection.DeleteOne(ctx, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while deleting exercise"})
			return
		}
		if result.DeletedCount < 1 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Exercise not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Exercise deleted successfully"})
	}
}
