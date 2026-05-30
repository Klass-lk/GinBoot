package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func main() {
	tableName := flag.String("table", "", "DynamoDB table name to migrate")
	region := flag.String("region", "ap-southeast-1", "AWS Region")
	dryRun := flag.Bool("dry-run", false, "If true, only prints what would be migrated without writing")
	flag.Parse()

	if *tableName == "" {
		log.Fatal("Please provide a -table name")
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	log.Printf("Starting migration for table: %s (Dry Run: %v)", *tableName, *dryRun)

	migratedCount := 0
	skippedCount := 0

	paginator := dynamodb.NewScanPaginator(client, &dynamodb.ScanInput{
		TableName: aws.String(*tableName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("failed to scan page, %v", err)
		}

		for _, item := range page.Items {
			// Check if item has legacy 'data' field
			dataAttr, ok := item["data"]
			if !ok {
				skippedCount++
				continue
			}

			strVal, ok := dataAttr.(*types.AttributeValueMemberS)
			if !ok || strVal.Value == "" {
				skippedCount++
				continue
			}

			// Parse the legacy JSON string into a generic map
			var entity map[string]interface{}
			if err := json.Unmarshal([]byte(strVal.Value), &entity); err != nil {
				log.Printf("Failed to unmarshal JSON for PK: %s, SK: %s - %v",
					getStringAttr(item, "pk"), getStringAttr(item, "sk"), err)
				skippedCount++
				continue
			}

			// Marshal the generic map into DynamoDB native attributes
			nativeItem, err := attributevalue.MarshalMap(entity)
			if err != nil {
				log.Printf("Failed to marshal native attributes for PK: %s, SK: %s - %v",
					getStringAttr(item, "pk"), getStringAttr(item, "sk"), err)
				skippedCount++
				continue
			}

			// Inject required base attributes from the original item
			for _, key := range []string{"pk", "sk", "id", "createdAt", "updatedAt", "version", "ttl"} {
				if val, exists := item[key]; exists {
					nativeItem[key] = val
				}
			}

			// Ensure 'data' field is removed to save storage & enable native queries
			delete(nativeItem, "data")

			if !*dryRun {
				// Save the item back to DynamoDB
				_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
					TableName: aws.String(*tableName),
					Item:      nativeItem,
				})
				if err != nil {
					log.Printf("Failed to update item PK: %s, SK: %s - %v",
						getStringAttr(item, "pk"), getStringAttr(item, "sk"), err)
					continue
				}
			}

			migratedCount++
			if migratedCount%100 == 0 {
				log.Printf("Migrated %d items...", migratedCount)
			}
			
			// Small sleep to prevent exceeding WCU burst capacity
			time.Sleep(10 * time.Millisecond)
		}
	}

	log.Printf("Migration completed! Migrated: %d, Skipped: %d", migratedCount, skippedCount)
}

func getStringAttr(item map[string]types.AttributeValue, key string) string {
	if val, ok := item[key]; ok {
		if s, ok := val.(*types.AttributeValueMemberS); ok {
			return s.Value
		}
	}
	return "UNKNOWN"
}
