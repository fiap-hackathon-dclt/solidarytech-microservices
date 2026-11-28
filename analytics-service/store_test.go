package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeDynamo struct {
	in  *dynamodb.TransactWriteItemsInput
	err error
}

func (f *fakeDynamo) TransactWriteItems(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.in = in
	return &dynamodb.TransactWriteItemsOutput{}, f.err
}

func (f *fakeDynamo) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return &dynamodb.QueryOutput{}, nil
}

func TestRecordKeys(t *testing.T) {
	db := &fakeDynamo{}
	s := &DynamoStore{db: db, table: "SolidaryTechAnalytics"}
	ev := DonationEvent{ID: 7, NgoID: 1, Amount: "10.56"}

	if err := s.Record(context.Background(), ev, "2026-11-28", "Educação"); err != nil {
		t.Fatal(err)
	}

	items := db.in.TransactItems
	if len(items) != 3 {
		t.Fatalf("itens na transação = %d, esperado 3", len(items))
	}
	if pk := attrS(items[0].Put.Item["PK"]); pk != "DOACAO#7" {
		t.Errorf("marcador PK = %s", pk)
	}
	if aws.ToString(items[0].Put.ConditionExpression) != "attribute_not_exists(PK)" {
		t.Error("o marcador precisa da condição de não existir")
	}
	if pk, sk := attrS(items[1].Update.Key["PK"]), attrS(items[1].Update.Key["SK"]); pk != "ONG#1" || sk != "DIA#2026-11-28" {
		t.Errorf("total da ONG = %s · %s", pk, sk)
	}
	if pk, sk := attrS(items[2].Update.Key["PK"]), attrS(items[2].Update.Key["SK"]); pk != "DIA#2026-11-28" || sk != "CAUSA#Educação" {
		t.Errorf("total da causa = %s · %s", pk, sk)
	}
	if v := attrN(items[2].Update.ExpressionAttributeValues[":valor"]); v != "10.56" {
		t.Errorf("valor somado = %s, esperado 10.56", v)
	}
}

func TestRecordDuplicate(t *testing.T) {
	db := &fakeDynamo{err: &dtypes.TransactionCanceledException{
		CancellationReasons: []dtypes.CancellationReason{
			{Code: aws.String("ConditionalCheckFailed")},
			{Code: aws.String("None")},
			{Code: aws.String("None")},
		},
	}}
	s := &DynamoStore{db: db, table: "SolidaryTechAnalytics"}

	err := s.Record(context.Background(), DonationEvent{ID: 7, NgoID: 1, Amount: "10.56"}, "2026-11-28", "Educação")
	if !errors.Is(err, ErrAlreadyRecorded) {
		t.Errorf("erro = %v, esperado ErrAlreadyRecorded", err)
	}
}
