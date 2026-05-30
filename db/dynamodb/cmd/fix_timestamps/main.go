package main

import (
	"context"
	"flag"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func main() {
	tableName := flag.String("table", "", "DynamoDB table name to fix timestamps on")
	region := flag.String("region", "ap-southeast-1", "AWS Region")
	dryRun := flag.Bool("dry-run", false, "If true, only prints what would be migrated without writing")
	flag.Parse()

	if *tableName == "" {
		log.Fatal("Please provide a -table name")
	}

	ctx := context.Background()
	
	// Configure SDK with higher retries
	cfg, err := config.LoadDefaultConfig(ctx, 
		config.WithRegion(*region),
		config.WithRetryMaxAttempts(10),
	)
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	client := dynamodb.NewFromConfig(cfg)
	log.Printf("Starting timestamp fix for table: %s (Dry Run: %v)", *tableName, *dryRun)

	fixedCount := 0
	skippedCount := 0
	var batch []types.WriteRequest

	paginator := dynamodb.NewScanPaginator(client, &dynamodb.ScanInput{
		TableName: aws.String(*tableName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("failed to scan page, %v", err)
		}

		for _, item := range page.Items {
			needsFix := false

			for _, key := range []string{"createdAt", "updatedAt"} {
				if val, exists := item[key]; exists {
					if nAttr, ok := val.(*types.AttributeValueMemberN); ok && len(nAttr.Value) >= 13 {
						if ms, err := strconv.ParseInt(nAttr.Value, 10, 64); err == nil {
							sec := ms / 1000
							item[key] = &types.AttributeValueMemberN{Value: strconv.FormatInt(sec, 10)}
							needsFix = true
						}
					}
				}
			}

			if !needsFix {
				skippedCount++
				continue
			}

			if !*dryRun {
				batch = append(batch, types.WriteRequest{
					PutRequest: &types.PutRequest{
						Item: item,
					},
				})

				if len(batch) == 25 {
					if err := processBatch(ctx, client, *tableName, batch); err != nil {
						log.Printf("Batch write failed: %v", err)
					}
					fixedCount += len(batch)
					log.Printf("Fixed timestamps for %d items...", fixedCount)
					batch = batch[:0]
					
					// Small delay to prevent crushing the GSI
					time.Sleep(100 * time.Millisecond)
				}
			} else {
				fixedCount++
			}
		}
	}

	// Process remaining batch
	if !*dryRun && len(batch) > 0 {
		if err := processBatch(ctx, client, *tableName, batch); err != nil {
			log.Printf("Final batch write failed: %v", err)
		}
		fixedCount += len(batch)
	}

	log.Printf("Timestamp fix completed! Fixed: %d, Skipped: %d", fixedCount, skippedCount)
}

func processBatch(ctx context.Context, client *dynamodb.Client, tableName string, batch []types.WriteRequest) error {
	unprocessed := map[string][]types.WriteRequest{
		tableName: batch,
	}

	backoff := 50 * time.Millisecond

	for len(unprocessed) > 0 {
		out, err := client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: unprocessed,
		})
		if err != nil {
			return err
		}

		if len(out.UnprocessedItems) > 0 {
			unprocessed = out.UnprocessedItems
			log.Printf("Throttled by DynamoDB (%d items unprocessed). Backing off for %v...", len(unprocessed[tableName]), backoff)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff up to a reasonable limit
			if backoff > 2*time.Second {
				backoff = 2 * time.Second
			}
		} else {
			unprocessed = nil
		}
	}
	return nil
}
