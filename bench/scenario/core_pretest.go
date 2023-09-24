package scenario

import (
	"context"
	"time"

	"github.com/isucon/isucon13/bench/isupipe"
)

func Pretest(ctx context.Context, client *isupipe.Client) error {
	if err := client.PostUser(ctx, &isupipe.PostUserRequest{
		Name:        "test",
		DisplayName: "test",
		Description: "blah blah blah",
		Password:    "s3cr3t",
	}); err != nil {
		return err
	}
	if err := client.Login(ctx, &isupipe.LoginRequest{
		UserName: "test",
		Password: "s3cr3t",
	}); err != nil {
		return err
	}
	if err := client.GetUser(ctx, "1" /* user id */); err != nil {
		return err
	}

	if err := client.GetTags(ctx); err != nil {
		return err
	}

	if err := client.ReserveLivestream(ctx, &isupipe.ReserveLivestreamRequest{
		Title:         "test",
		Description:   "test",
		PrivacyStatus: "public",
		StartAt:       time.Now().Unix(),
		EndAt:         time.Now().Unix(),
	}); err != nil {
		return err
	}

	if err := client.PostSuperchat(ctx, 1, &isupipe.PostSuperchatRequest{
		Comment: "test",
		Tip:     3,
	}); err != nil {
		return err
	}

	if err := client.GetLivestreamsByTag(ctx, "chair" /* tag name */); err != nil {
		return err
	}

	return nil
}
