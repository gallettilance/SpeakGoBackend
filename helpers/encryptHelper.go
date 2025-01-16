package helpers

import (
	"context"
	"log"
	"encoding/base64"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	// "github.com/joho/godotenv"
)

func WrapKey(key, kmsKeyID string) (string, error) {
	kmsClient := GetKMSClient()

	input := &kms.EncryptInput{
		KeyId:     aws.String(kmsKeyID),
		Plaintext: []byte(key),
	}

	result, err := kmsClient.Encrypt(context.Background(), input)
	if err != nil {
		log.Printf("Error encrypting key: %v", err)
		return "", err
	}

	return base64.StdEncoding.EncodeToString(result.CiphertextBlob), nil
}

func UnwrapKey(wrappedKey, kmsKeyID string) (string, error) {
	kmsClient := GetKMSClient()

	ciphertextBlob, err := base64.StdEncoding.DecodeString(wrappedKey)
	if err != nil {
		log.Printf("Error decoding wrapped key: %v", err)
		return "", err
	}

	input := &kms.DecryptInput{
		KeyId:             aws.String(kmsKeyID),
		CiphertextBlob:    ciphertextBlob,
		EncryptionContext: nil, // Add encryption context if used during wrapping
	}

	result, err := kmsClient.Decrypt(context.Background(), input)
	if err != nil {
		log.Printf("Error decrypting key: %v", err)
		return "", err
	}

	return string(result.Plaintext), nil
}