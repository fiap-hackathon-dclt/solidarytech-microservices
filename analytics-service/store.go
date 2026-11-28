package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Maior que a retenção máxima do SQS (14 dias): duplicata nenhuma chega depois que o marcador expira.
const markerTTL = 15 * 24 * time.Hour

var ErrAlreadyRecorded = errors.New("doação já somada")

type DynamoAPI interface {
	TransactWriteItems(ctx context.Context, in *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	dynamodb.QueryAPIClient
}

type DynamoStore struct {
	db    DynamoAPI
	table string
}

type CauseTotal struct {
	Cause     string      `json:"causa"`
	Total     json.Number `json:"total"`
	Donations json.Number `json:"doacoes"`
}

type DayTotal struct {
	Day       string      `json:"dia"`
	Total     json.Number `json:"total"`
	Donations json.Number `json:"doacoes"`
}

func (s *DynamoStore) Record(ctx context.Context, ev DonationEvent, day, cause string) error {
	expiresAt := time.Now().Add(markerTTL).Unix()
	_, err := s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []dtypes.TransactWriteItem{
			{Put: &dtypes.Put{
				TableName: aws.String(s.table),
				Item: map[string]dtypes.AttributeValue{
					"PK":        str(fmt.Sprintf("DOACAO#%d", ev.ID)),
					"SK":        str("MARCADOR"),
					"expira_em": num(strconv.FormatInt(expiresAt, 10)),
				},
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}},
			{Update: s.add(fmt.Sprintf("ONG#%d", ev.NgoID), "DIA#"+day, ev.Amount)},
			{Update: s.add("DIA#"+day, "CAUSA#"+cause, ev.Amount)},
		},
	})

	var canceled *dtypes.TransactionCanceledException
	if errors.As(err, &canceled) && len(canceled.CancellationReasons) > 0 &&
		aws.ToString(canceled.CancellationReasons[0].Code) == "ConditionalCheckFailed" {
		return ErrAlreadyRecorded
	}
	return err
}

func (s *DynamoStore) add(pk, sk string, amount json.Number) *dtypes.Update {
	return &dtypes.Update{
		TableName:                aws.String(s.table),
		Key:                      map[string]dtypes.AttributeValue{"PK": str(pk), "SK": str(sk)},
		UpdateExpression:         aws.String("ADD #total :valor, #doacoes :um"),
		ExpressionAttributeNames: map[string]string{"#total": "total", "#doacoes": "doacoes"},
		ExpressionAttributeValues: map[string]dtypes.AttributeValue{
			":valor": num(amount.String()),
			":um":    num("1"),
		},
	}
}

func (s *DynamoStore) CauseTotals(ctx context.Context, day string) ([]CauseTotal, error) {
	items, err := s.partition(ctx, "DIA#"+day)
	if err != nil {
		return nil, err
	}
	totals := make([]CauseTotal, 0, len(items))
	for _, it := range items {
		totals = append(totals, CauseTotal{
			Cause:     strings.TrimPrefix(attrS(it["SK"]), "CAUSA#"),
			Total:     json.Number(attrN(it["total"])),
			Donations: json.Number(attrN(it["doacoes"])),
		})
	}
	sort.SliceStable(totals, func(i, j int) bool {
		return asFloat(totals[i].Total) > asFloat(totals[j].Total)
	})
	return totals, nil
}

func (s *DynamoStore) NGOTotals(ctx context.Context, ngoID int) ([]DayTotal, error) {
	items, err := s.partition(ctx, fmt.Sprintf("ONG#%d", ngoID))
	if err != nil {
		return nil, err
	}
	totals := make([]DayTotal, 0, len(items))
	for _, it := range items {
		totals = append(totals, DayTotal{
			Day:       strings.TrimPrefix(attrS(it["SK"]), "DIA#"),
			Total:     json.Number(attrN(it["total"])),
			Donations: json.Number(attrN(it["doacoes"])),
		})
	}
	return totals, nil
}

func (s *DynamoStore) partition(ctx context.Context, pk string) ([]map[string]dtypes.AttributeValue, error) {
	pages := dynamodb.NewQueryPaginator(s.db, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		KeyConditionExpression:    aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]dtypes.AttributeValue{":pk": str(pk)},
	})
	var items []map[string]dtypes.AttributeValue
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
	}
	return items, nil
}

func str(v string) dtypes.AttributeValue {
	return &dtypes.AttributeValueMemberS{Value: v}
}

func num(v string) dtypes.AttributeValue {
	return &dtypes.AttributeValueMemberN{Value: v}
}

func attrS(v dtypes.AttributeValue) string {
	if s, ok := v.(*dtypes.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

func attrN(v dtypes.AttributeValue) string {
	if n, ok := v.(*dtypes.AttributeValueMemberN); ok {
		return n.Value
	}
	return "0"
}

func asFloat(n json.Number) float64 {
	f, _ := n.Float64()
	return f
}
