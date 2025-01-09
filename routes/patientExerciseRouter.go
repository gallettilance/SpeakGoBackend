package routes

import(
	controller "golang-speakbackend/controllers"

	"github.com/gin-gonic/gin"
)

func PatientExerciseRoutes(incomingRoutes *gin.RouterGroup){
	incomingRoutes.GET("/patientexercise/:id", controller.GetPatientExercise())
	// incomingRoutes.GET("/patientexercises", controller.GetPatientExercises())
	incomingRoutes.POST("/patientexercise", controller.CreatePatientExercise())
	incomingRoutes.PUT("/patientexercise/:id", controller.UpdatePatientExercise())
	incomingRoutes.DELETE("/patientexercise/:id", controller.DeletePatientExercise())
	incomingRoutes.POST("/patientexercise/uploadrecording/:patient_exercise_id", controller.UploadRecording())
	incomingRoutes.GET("/patientexercises/:patient_id", controller.GetPatientExercisesByUser())
	incomingRoutes.POST("/getuploadurl/:patient_exercise_id", controller.RecordingPresignPost())
	incomingRoutes.GET("/getdownloadurl/:patient_exercise_id", controller.GetRecordingPresignURL())
}