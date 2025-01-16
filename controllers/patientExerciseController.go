package controllers

import (
	"context"
	"fmt"
	"golang-speakbackend/database"
	"golang-speakbackend/helpers"
	"golang-speakbackend/models"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var patientExerciseCollection *mongo.Collection = database.OpenCollection(database.Client, "patient_exercise")

// func wrapKey(key, kmsKeyID string) (string, error) {
// 	kmsClient := helpers.GetKMSClient()

// 	input := &kms.EncryptInput{
// 		KeyId:     aws.String(kmsKeyID),
// 		Plaintext: []byte(key),
// 	}

// 	result, err := kmsClient.Encrypt(context.Background(), input)
// 	if err != nil {
// 		log.Printf("Error encrypting key: %v", err)
// 		return "", err
// 	}

// 	return base64.StdEncoding.EncodeToString(result.CiphertextBlob), nil
// }

// func unwrapKey(wrappedKey, kmsKeyID string) (string, error) {
// 	kmsClient := helpers.GetKMSClient()

// 	ciphertextBlob, err := base64.StdEncoding.DecodeString(wrappedKey)
// 	if err != nil {
// 		log.Printf("Error decoding wrapped key: %v", err)
// 		return "", err
// 	}

// 	input := &kms.DecryptInput{
// 		KeyId:             aws.String(kmsKeyID),
// 		CiphertextBlob:    ciphertextBlob,
// 		EncryptionContext: nil, // Add encryption context if used during wrapping
// 	}

// 	result, err := kmsClient.Decrypt(context.Background(), input)
// 	if err != nil {
// 		log.Printf("Error decrypting key: %v", err)
// 		return "", err
// 	}

// 	return string(result.Plaintext), nil
// }

func RecordingPresignPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		s3Client := helpers.GetS3Client()
		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		patientExerciseID := c.Param("patient_exercise_id")

		type RequestBody struct {
			AESKey string `json:"aes_key"`
		}

		var requestBody RequestBody
		if err := c.BindJSON(&requestBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		// Wrap the AES key using AWS KMS
		kmsKeyID := os.Getenv("KMS_KEY_ID")
		wrappedKey, err := helpers.WrapKey(requestBody.AESKey, kmsKeyID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to wrap encryption key"})
			return
		}

		// Generate signed URL with necessary headers
		presignClient := s3.NewPresignClient(s3Client)

		presignedURL, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("peakspeak"),
			Key:    aws.String(fmt.Sprintf("recordings/%s.mp4", patientExerciseID)),
		}, s3.WithPresignExpires(15*time.Minute))

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate signed URL"})
			return
		}

		// Store the wrapped key and other metadata
		updatedAt, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "wrapped_key", Value: wrappedKey},
				{Key: "updated_at", Value: updatedAt},
				{Key: "status", Value: "completed"},
			}},
		}

		upsert := true

		opt := options.UpdateOptions{
			Upsert: &upsert,
		}
		_, err = patientExerciseCollection.UpdateOne(
			ctx,
			bson.M{"patient_exercise_id": patientExerciseID},
			update,
			&opt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update patient exercise with wrapped key"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"upload_url": presignedURL.URL})
	}
}

func GetRecordingPresignURL() gin.HandlerFunc {
	return func(c *gin.Context) {
		s3Client := helpers.GetS3Client()
		kmsKeyID := os.Getenv("KMS_KEY_ID")
		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		patientExerciseID := c.Param("patient_exercise_id")

		// Retrieve the wrapped key and other metadata from your database
		var patientExercise models.PatientExercise
		err := patientExerciseCollection.FindOne(ctx, bson.M{"patient_exercise_id": patientExerciseID}).Decode(&patientExercise)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Patient exercise not found"})
			return
		}

		// Unwrap the AES key using AWS KMS
		unwrappedKey, err := helpers.UnwrapKey(patientExercise.WrappedKey, kmsKeyID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unwrap encryption key"})
			return
		}

		// Generate a pre-signed URL for the GET request
		presignClient := s3.NewPresignClient(s3Client)

		presignedURL, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String("peakspeak"),
			Key:    aws.String(fmt.Sprintf("recordings/%s.mp4", patientExerciseID)),
		}, s3.WithPresignExpires(15*time.Minute))

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate signed URL"})
			return
		}

		// Return the pre-signed URL and the unwrapped AES key for downloading and decrypting the video
		c.JSON(http.StatusOK, gin.H{
			"download_url": presignedURL.URL,
			"aes_key":      unwrappedKey,
		})
	}
}

func UploadRecording() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		patientExerciseID := c.Param("patient_exercise_id")
		var patientExercise models.PatientExercise
		err := patientExerciseCollection.FindOne(ctx, bson.M{"patient_exercise_id": patientExerciseID}).Decode(&patientExercise)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Patient exercise not found"})
			return
		}

		err = c.Request.ParseMultipartForm(100 << 20) // 100 MB
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse multipart form"})
			return
		}

		file, handler, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Error occurred while uploading file"})
			return
		}
		defer file.Close()

		fmt.Printf("Uploaded File: %+v\n", handler.Filename)
		fmt.Printf("File Size: %+v\n", handler.Size)
		fmt.Printf("MIME Header: %+v\n", handler.Header)

		fileExtension := strings.Split(handler.Filename, ".")[1]

		// Key should be recordings/patientExerciseID.extension
		key := fmt.Sprintf("recordings/%s.%s", patientExerciseID, fileExtension)

		// Upload the file to S3
		err = helpers.UploadFileToS3(ctx, helpers.GetS3Client(), "peakspeak", key, file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upload file, %v", err)})
			return
		}

		videoURL := fmt.Sprintf("https://peakspeak.nyc3.cdn.digitaloceanspaces.com/recordings/%s.%s", patientExerciseID, fileExtension)
		updatedAt := time.Now().Format(time.RFC3339)
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "recording", Value: videoURL},
				{Key: "updated_at", Value: updatedAt},
				{Key: "status", Value: "completed"},
			}},
		}

		_, err = patientExerciseCollection.UpdateOne(ctx, bson.M{"patient_exercise_id": patientExerciseID}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while updating patient exercise recording"})
			return
		}

		fmt.Printf("File uploaded to, %s\n", videoURL)

		c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully", "location": videoURL})
	}
}

func GetPatientExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		patientExerciseID := c.Param("patient_exercise_id")
		var patientExercise models.PatientExercise

		err := patientExerciseCollection.FindOne(ctx, bson.M{"patient_exercise_id": patientExerciseID}).Decode(&patientExercise)
		defer cancel()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching exercise"})
			return
		}
		c.JSON(http.StatusOK, patientExercise)
	}
}

func GetPatientExercisesByUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get patient user ID from the URL parameter
		patientID := c.Param("patient_id")
		if patientID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Patient ID is required"})
			return
		}

		// Find all patient exercises for the given patient ID
		filter := bson.M{"patient_id": patientID}
		result, err := patientExerciseCollection.Find(ctx, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching patient exercises"})
			return
		}

		var patientExercises []bson.M
		if err = result.All(ctx, &patientExercises); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while decoding patient exercises"})
			return
		}

		// Fetch exercise details for each patient exercise
		var detailedExercises []bson.M
		for _, patientExercise := range patientExercises {
			exerciseID := patientExercise["exercise_id"].(string)
			var exercise bson.M
			err = exerciseCollection.FindOne(ctx, bson.M{"exercise_id": exerciseID}).Decode(&exercise)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Error while fetching exercise details"})
				return
			}

			// Ensure the exercise has an ID field
			exerciseIDField, exists := exercise["_id"]
			if !exists {
				exerciseIDField = exerciseID
			}

			detailedExercise := bson.M{
				"id":               patientExercise["_id"],
				"patient_exercise": patientExercise,
				"exercise":         exercise,
				"exercise_id":      exerciseIDField,
			}
			detailedExercises = append(detailedExercises, detailedExercise)
		}

		c.JSON(http.StatusOK, detailedExercises)
	}
}

func ExercisesByPatient(id string) (PatientExercises []primitive.M, err error) {
	var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

	matchStage := bson.D{{Key: "$match", Value: bson.D{{Key: "patient_exercise_id", Value: id}}}}
	lookupStage := bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "patient"}, {Key: "localField", Value: "patient_id"}, {Key: "foreignField", Value: "patient_id"}, {Key: "as", Value: "patient"}}}}
	unwindStage := bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$patient"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}}

	lookupPatientExerciseStage := bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "patient_exercise"}, {Key: "localField", Value: "patient_exercise_id"}, {Key: "foreignField", Value: "patient_exercise_id"}, {Key: "as", Value: "patient_exercise"}}}}
	unwindPatientExerciseStage := bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$patient_exercise"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}}

	lookupPatientStage := bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "user"}, {Key: "localField", Value: "patient_exercise.patient_id"}, {Key: "foreignField", Value: "user_id"}, {Key: "as", Value: "patient"}}}}
	unwindPatientStage := bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$patient"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}}

	lookupTherapistStage := bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "user"}, {Key: "localField", Value: "patient_exercise.therapist_id"}, {Key: "foreignField", Value: "user_id"}, {Key: "as", Value: "therapist"}}}}
	unwindTherapistStage := bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$therapist"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}}

	lookupExerciseStage := bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "exercise"}, {Key: "localField", Value: "patient_exercise.exercise_id"}, {Key: "foreignField", Value: "exercise_id"}, {Key: "as", Value: "exercise"}}}}
	unwindExerciseStage := bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$exercise"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}}

	projectStage := bson.D{
		{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "exercise_name", Value: "$exercise.name"},
			{Key: "exercise_description", Value: "$exercise.description"},
			{Key: "exercise_video_url", Value: "$exercise.video_url"},
			{Key: "patient_id", Value: "$patient.user_id"},
			{Key: "patient_name", Value: "$patient.name"},
			{Key: "therapist_id", Value: "$therapist.user_id"},
			{Key: "therapist_name", Value: "$therapist.name"},
			{Key: "patient_exercise_id", Value: "$patient_exercise.patient_exercise_id"},
			{Key: "patient_exercise_status", Value: "$patient_exercise.status"},
		}}}

	groupStage := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "patient_exercise_id", Value: "$patient_exercise_id"}, {Key: "patient_id", Value: "$patient_id"}, {Key: "therapist_id", Value: "$therapist_id"}, {Key: "exercise_id", Value: "$exercise_id"}}}, {Key: "exercises", Value: bson.D{{Key: "$push", Value: "$$ROOT"}}}}}}

	projectStage2 := bson.D{
		{Key: "$project", Value: bson.D{
			{Key: "id", Value: 0},
			{Key: "patient_exercise_id", Value: "$_id.patient_exercise_id"},
			{Key: "patient_id", Value: "$_id.patient_id"},
			{Key: "therapist_id", Value: "$_id.therapist_id"},
			{Key: "exercise_id", Value: "$_id.exercise_id"},
			{Key: "exercises", Value: 1},
		}}}

	result, err := patientExerciseCollection.Aggregate(ctx, mongo.Pipeline{
		matchStage,
		lookupStage,
		unwindStage,
		lookupPatientExerciseStage,
		unwindPatientExerciseStage,
		lookupPatientStage,
		unwindPatientStage,
		lookupTherapistStage,
		unwindTherapistStage,
		lookupExerciseStage,
		unwindExerciseStage,
		projectStage,
		groupStage,
		projectStage2,
	})

	if err != nil {
		panic(err)
	}

	if err = result.All(ctx, &PatientExercises); err != nil {
		panic(err)
	}

	defer cancel()
	return PatientExercises, err
}

func CreatePatientExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var requestBody struct {
			PatientID   string   `json:"patient_id" validate:"required"`
			TherapistID string   `json:"therapist_id" validate:"required"`
			ExerciseIDs []string `json:"exercise_ids" validate:"required,dive,required"`
		}

		// var patient models.User
		// var therapist models.User

		if err := c.BindJSON(&requestBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		validationError := validate.Struct(requestBody)
		if validationError != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationError.Error()})
			return
		}

		// Fetch therapist and patient information
		var users []models.User
		userCursor, userErr := userCollection.Find(ctx, bson.M{
			"user_id": bson.M{"$in": []string{requestBody.TherapistID, requestBody.PatientID}},
		})
		if userErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching user data"})
			return
		}
		defer userCursor.Close(ctx)
		if err := userCursor.All(ctx, &users); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error decoding user data"})
			return
		}

		var therapist, patient *models.User
		for _, user := range users {
			if user.UserID == requestBody.TherapistID {
				therapist = &user
			}
			if user.UserID == requestBody.PatientID {
				patient = &user
			}
		}

		if therapist == nil || patient == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Therapist or Patient not found"})
			return
		}

		// Check if the therapist has the role of 'therapist'
		if therapist.Role != "therapist" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Only users with the role of therapist can create patient exercises"})
			return
		}

		// Validate Patient ID
		// patientErr := userCollection.FindOne(ctx, bson.M{"user_id": requestBody.PatientID}).Decode(&patient)
		// if patientErr != nil {
		// 	msg := fmt.Sprintf("Patient with ID %s not found", *patientExercise.PatientID)
		// 	c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
		// 	return
		// }

		// Fetch exercises
		var exercises []models.Exercise
		exerciseCursor, exerciseErr := exerciseCollection.Find(ctx, bson.M{"exercise_id": bson.M{"$in": requestBody.ExerciseIDs}})
		if exerciseErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching exercise data"})
			return
		}
		defer exerciseCursor.Close(ctx)
		if err := exerciseCursor.All(ctx, &exercises); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error decoding exercise data"})
			return
		}

		if len(exercises) != len(requestBody.ExerciseIDs) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Some exercises not found"})
			return
		}

		// Create Patient Exercise documents
		var patientExercises []interface{}
		for _, exerciseID := range requestBody.ExerciseIDs {
			created_at, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
			updated_at, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
			patientExercise := models.PatientExercise{
				ID:          primitive.NewObjectID(),
				PatientID:   &requestBody.PatientID,
				TherapistID: &requestBody.TherapistID,
				ExerciseID:  &exerciseID,
				Status:      "pending",
				Recording:   "",
				CreatedAt:   created_at,
				UpdatedAt:   updated_at,
			}
			patientExercise.PatientExerciseID = patientExercise.ID.Hex()
			patientExercises = append(patientExercises, patientExercise)
		}

		insertResult, insertErr := patientExerciseCollection.InsertMany(ctx, patientExercises)
		if insertErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error inserting patient exercises"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"inserted_ids": insertResult.InsertedIDs})
	}
}

func UpdatePatientExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		patientExerciseID := c.Param("patient_exercise_id")
		if patientExerciseID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Patient exercise ID is required"})
			return
		}

		var patientExercise models.PatientExercise
		if err := c.BindJSON(&patientExercise); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var updateObj primitive.D

		if patientExercise.Status != "" {
			updateObj = append(updateObj, bson.E{Key: "status", Value: patientExercise.Status})
		}

		if patientExercise.Recording != "" {
			updateObj = append(updateObj, bson.E{Key: "recording", Value: patientExercise.Recording})
		}

		patientExercise.UpdatedAt = time.Now()
		updateObj = append(updateObj, bson.E{Key: "updated_at", Value: patientExercise.UpdatedAt})

		filter := bson.M{"patient_exercise_id": patientExerciseID}
		update := bson.D{
			{Key: "$set", Value: updateObj},
		}

		result, err := patientExerciseCollection.UpdateOne(ctx, filter, update)
		if err != nil {
			msg := fmt.Sprintf("Error while updating patient exercise: %s", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			return
		}

		if result.MatchedCount == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Patient exercise not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Patient exercise updated successfully", "result": result})
	}
}

func DeletePatientExercise() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		patientExerciseID := c.Param("id")
		// turn patientExerciseID into an ObjectID
		objID, err := primitive.ObjectIDFromHex(patientExerciseID)
		defer cancel()
		if err != nil {
			msg := fmt.Sprintf("Object ID %s is invalid", patientExerciseID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			return
		}

		// delete the patient exercise
		_, err = patientExerciseCollection.DeleteOne(ctx, bson.M{"_id": objID})
		if err != nil {
			msg := fmt.Sprintf("Error while deleting patient exercise: %s", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			return
		}

		// _, err := patientExerciseCollection.DeleteOne(ctx, bson.M{"_id": objID})
		// defer cancel()
		// if err != nil {
		// 	msg := fmt.Sprintf("Error while deleting exercise: %s", err)
		// 	c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
		// 	return
		// }

		c.JSON(http.StatusOK, gin.H{"message": "Deleted exercise successfully"})
	}
}
