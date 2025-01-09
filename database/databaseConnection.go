package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	// "github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DBInstance initializes and returns a MongoDB client
func DBInstance() *mongo.Client {
	// err := godotenv.Load()
	// if err != nil {
	//     log.Fatalf("Error loading .env file")
	// }

	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	// It's highly recommended to store your URI securely, e.g., using environment variables
	uri := os.Getenv("MONGO_URI")
	opts := options.Client().ApplyURI(uri).SetServerAPIOptions(serverAPI)

	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Send a ping to confirm a successful connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err(); err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}

	fmt.Println("Successfully connected and pinged your deployment.")
	return client
}

var Client *mongo.Client = DBInstance()

// OpenCollection returns a reference to a MongoDB collection and ensures indexes are created
func OpenCollection(client *mongo.Client, collectionName string) *mongo.Collection {
	collection := client.Database("golang-speakdb").Collection(collectionName)
	createIndexes(collection)
	return collection
}

// createIndexes creates necessary indexes on the collection
func createIndexes(collection *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "is_global", Value: 1}}, // Index on is_global
			Options: options.Index().SetUnique(false),
		},
		{
			Keys: bson.D{{Key: "therapist_id", Value: 1}}, // Index on therapist_id
			Options: options.Index().SetUnique(false),
		},
	}

	indexNames, err := collection.Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		log.Fatalf("Failed to create indexes on collection %s: %v", collection.Name(), err)
	}

	fmt.Printf("Indexes created on collection %s: %v\n", collection.Name(), indexNames)
}
