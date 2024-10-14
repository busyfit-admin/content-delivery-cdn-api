package main

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodb_attributevalue "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodb_types "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	sign "github.com/aws/aws-sdk-go/service/cloudfront/sign"
)

type CardsTable struct {
	CardId         string `dynamodbav:"CardId" json:"CardId"`           // Unique CardId
	CardName       string `dynamodbav:"CardName" json:"CardName"`       // Custom name of the card that is entered by the company
	CardS3Location string `dynamodbav:"CardType" json:"CardS3Location"` // S3 Location of the Card
}

type CardsTableTemplateService struct {
	ctx    context.Context
	logger *log.Logger

	dynamodbClient   DynamodbClient
	s3ObjectClient   S3Client
	cloudfrontClient CloudfrontClient
	secretMgrClient  SecretManagerClient

	CompanyCardTemplateTable string
	BucketName               string

	publicKeyId string
	privateKey  *rsa.PrivateKey
}

func (svc *CardsTableTemplateService) AssignCardsService(
	ctx context.Context,
	logger *log.Logger,

	ddbClient DynamodbClient,
	s3Client S3Client,
	secretMgrClient SecretManagerClient,

	PKsSecretKeyArn string,
	publicKeyId string,

) error {

	err := svc.AssignPrivatePublicKey(PKsSecretKeyArn, publicKeyId)
	if err != nil {
		return err
	}

	signerClient := sign.NewURLSigner(svc.publicKeyId, svc.privateKey)

	// Assign Clients
	svc.ctx = ctx
	svc.dynamodbClient = ddbClient
	svc.logger = logger
	svc.s3ObjectClient = s3Client
	svc.cloudfrontClient = signerClient
	svc.secretMgrClient = secretMgrClient

	return nil

}
func (svc *CardsTableTemplateService) AssignPrivatePublicKey(secretKeyArn string, publicKeyId string) error {

	if secretKeyArn == "" {
		return fmt.Errorf("[ERROR] Secret ARN cannot be empty!")
	}

	input := secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretKeyArn),
	}

	output, err := svc.secretMgrClient.GetSecretValue(svc.ctx, &(input))
	if err != nil {
		svc.logger.Printf("[ERROR] Failed to retrieve data from the secret manager. Error : %v", err)
		return err
	}

	privateKeyString := *output.SecretString

	if privateKeyString == "" {
		svc.logger.Printf("[ERROR] PrivateKey Value is empty : %v ", privateKeyString)
		return fmt.Errorf("[ERROR] PrivateKey Value is empty : %v ", privateKeyString)
	}

	// Parse Private Key
	parsedPrivateKey, err := parsePrivateKey(privateKeyString)
	if err != nil {
		return err
	}
	// assign Private Key in Service
	svc.privateKey = parsedPrivateKey

	svc.logger.Printf("Assign Private Key Completed")

	// assign Public Key ID of the CloudFront
	svc.publicKeyId = publicKeyId

	svc.logger.Printf("Assign Public Key Completed")

	return nil
}
func parsePrivateKey(pemPrivateKey string) (*rsa.PrivateKey, error) {
	// Trim any leading/trailing whitespaces
	pemPrivateKey = strings.TrimSpace(pemPrivateKey)

	// Parse the PEM block containing the private key
	block, _ := pem.Decode([]byte(pemPrivateKey))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing private key")
	}

	// Parse the private key from the PEM block
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

func (svc *CardsTableTemplateService) PutObjectToS3(key string, imageData []byte) error {

	_, err := svc.s3ObjectClient.PutObject(svc.ctx, &s3.PutObjectInput{
		Bucket:      aws.String(svc.BucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(imageData),
		ContentType: aws.String("image/jpeg"),
	})

	if err != nil {
		svc.logger.Printf("Error uploading image to S3: %v\n", err)
		return err
	}

	svc.logger.Printf("Image uploaded successfully to S3. Object key: %s\n", key)

	return nil
}

func (svc *CardsTableTemplateService) PutMetaData(data CardsTable) error {

	UpdateItemInput := dynamodb.UpdateItemInput{
		TableName: aws.String(svc.CompanyCardTemplateTable),
		Key: map[string]dynamodb_types.AttributeValue{
			"CardId": &dynamodb_types.AttributeValueMemberS{Value: data.CardId},
		},
		UpdateExpression: aws.String("SET CardName = :CardName, CardS3Location = :CardS3Location"),
		ExpressionAttributeValues: map[string]dynamodb_types.AttributeValue{
			":CardName":       &dynamodb_types.AttributeValueMemberS{Value: data.CardName},
			":CardS3Location": &dynamodb_types.AttributeValueMemberS{Value: data.CardS3Location},
		},
		ReturnValues: dynamodb_types.ReturnValueNone,
	}

	_, err := svc.dynamodbClient.UpdateItem(svc.ctx, &UpdateItemInput)
	if err != nil {
		svc.logger.Printf("Failed to Update the Company Table: %v\n", err)
		return err
	}
	svc.logger.Printf("SUCCESS : Updated MetaData table with the new Card information")

	return nil
}

func (svc *CardsTableTemplateService) GetMetaData(CardId string) (CardsTable, error) {

	if CardId == "" {
		return CardsTable{}, fmt.Errorf("CardId cannot be empty for GetMeta Data for single Card")
	}

	getItemInput := dynamodb.GetItemInput{
		TableName: aws.String(svc.CompanyCardTemplateTable),
		Key: map[string]dynamodb_types.AttributeValue{
			"CardId": &dynamodb_types.AttributeValueMemberS{Value: CardId},
		},
		ConsistentRead: aws.Bool(true),
	}

	output, err := svc.dynamodbClient.GetItem(svc.ctx, &getItemInput)
	if err != nil {
		return CardsTable{}, err
	}

	var CardMetaData CardsTable
	err = dynamodb_attributevalue.UnmarshalMap(output.Item, &CardMetaData)
	if err != nil {
		svc.logger.Printf("Unmarshal on the CardMetaData Failed :%v", err)
		return CardsTable{}, err
	}

	return CardMetaData, nil
}

// Gets Presigned URL for the Card present in the CloudFront Location.
func (svc *CardsTableTemplateService) GetPreSignedURL(objectKey string) (string, error) {

	s3URL, err := GenerateS3URL(svc.BucketName, objectKey)
	if err != nil {
		return "", err
	}

	singedURL, err := svc.cloudfrontClient.Sign(s3URL, time.Now().Add(1*time.Hour))

	if err != nil {
		svc.logger.Printf("Unable to sign the request")
		return "", err
	}

	return singedURL, nil
}

// GenerateS3URL generates an S3 URL for a given bucket and key.
func GenerateS3URL(bucket, key string) (string, error) {
	if bucket == "" || key == "" {
		return "", fmt.Errorf("bucket and key must be provided")
	}

	// Encode the key to make it URL-safe
	encodedKey := url.PathEscape(key)

	// Format the S3 URL
	s3URL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, encodedKey)
	return s3URL, nil
}
