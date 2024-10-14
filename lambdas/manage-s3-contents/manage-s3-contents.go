package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-xray-sdk-go/instrumentation/awsv2"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/google/uuid"
)

var RESP_HEADERS = map[string]string{
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Methods": "*",
	"Access-Control-Allow-Headers": "card-name,X-Amz-Date,X-Api-Key,X-Amz-Security-Token,X-Requested-With,X-Auth-Token,Referer,User-Agent,Origin,Content-Type,Authorization,Accept,Access-Control-Allow-Methods,Access-Control-Allow-Origin,Access-Control-Allow-Headers",
}

type Service struct {
	ctx    context.Context
	logger *log.Logger

	SVC CardsTableTemplateService
}

func main() {

	ctx, root := xray.BeginSegment(context.TODO(), "manage-cards-template")
	defer root.Close(nil)

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v\n", err)
	}
	awsv2.AWSV2Instrumentor(&cfg.APIOptions)

	dynamodbClient := dynamodb.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg)
	logger := log.New(os.Stdout, "", log.LstdFlags)

	secretMgrClnt := secretsmanager.NewFromConfig(cfg)

	// Company SVC Initial setup Functions
	SVC := CardsTableTemplateService{
		CompanyCardTemplateTable: os.Getenv("COMPANY_CARDS_TABLE"),
		BucketName:               os.Getenv("COMPANY_CARDS_BUCKET"),
	}

	// Assigns all Clients and Gets Private, Public Key's and sets it in Company Service Struct
	err = SVC.AssignCardsService(
		ctx,
		logger,
		dynamodbClient,
		s3Client,
		secretMgrClnt,
		os.Getenv("PRIVATE_KEY_SECRET_MGR_ARN"),
		os.Getenv("PUBLIC_KEY_CLOUDFRONT_ID"),
	)
	if err != nil {
		log.Fatalf("Cannot Assign Clients in Company Svc: %v\n", err)
	}

	// Manage Cards Svc Setup
	svc := Service{
		ctx:    ctx,
		logger: logger,
		SVC:    *&SVC,
	}

	lambda.Start(svc.handleCardsEvents)
}

func (svc *Service) handleCardsEvents(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	switch request.HTTPMethod {

	case "POST":
		return svc.handlePostMethod(request)

	case "GET":
		return svc.handleGetMethod(request)

	default:
		svc.logger.Printf("Request type not defined for ManageCardTemplate: %s", request.HTTPMethod)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}

}

func (svc *Service) handlePostMethod(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// 1 ) Creating a new UUID for the Object Key
	newUUID := uuid.New().String()
	newUUID += ".jpeg"

	// 2) Upload the Image into the Cards S3 bucket
	// Process and store the image data in S3
	imageData := []byte(request.Body)
	err := svc.SVC.PutObjectToS3(newUUID, imageData)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Headers:    RESP_HEADERS,
			StatusCode: 500,
		}, err
	}

	// 3) Update the MetaData Table
	err = svc.SVC.PutMetaData(CardsTable{
		CardId:         newUUID,
		CardName:       request.Headers["card-name"],
		CardS3Location: newUUID,
	})
	if err != nil {
		return events.APIGatewayProxyResponse{
			Headers:    RESP_HEADERS,
			StatusCode: 500,
		}, err
	}

	// Return Success when all the above steps are completed
	return events.APIGatewayProxyResponse{
		Headers:    RESP_HEADERS,
		StatusCode: 200,
	}, nil
}

func (svc *Service) handleGetMethod(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// 1. Get Req Headers
	cardId := request.Headers["card-id"]

	// 2. Get the Card from the card template ddb
	cardMetaData, err := svc.SVC.GetMetaData(cardId)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Headers:    RESP_HEADERS,
			StatusCode: 500,
		}, err
	}

	// 3. Get Cards Presign URL
	preSignURL, err := svc.SVC.GetPreSignedURL(cardMetaData.CardId)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Headers:    RESP_HEADERS,
			StatusCode: 500,
		}, err
	}

	if preSignURL == "" {
		return events.APIGatewayProxyResponse{
			Headers:    RESP_HEADERS,
			StatusCode: 500,
		}, err
	}

	getReqRes := GETReqResponse{
		CardId:   cardMetaData.CardId,
		CardName: cardMetaData.CardName,

		CardTemplateUrl: preSignURL,
	}

	// Format the response
	respBytes, err := json.Marshal(getReqRes)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Headers:    RESP_HEADERS,
			StatusCode: 500,
		}, err
	}

	return events.APIGatewayProxyResponse{
		Body:       string(respBytes),
		StatusCode: 200,
		Headers:    RESP_HEADERS,
	}, nil
}

type GETReqResponse struct {
	CardId   string `json:"CardId"`
	CardName string `json:"CardName"`

	CardTemplateUrl string `json:"CardTemplateURL"`
}
