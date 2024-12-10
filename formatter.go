package main

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/go-github/v67/github"
	"go.uber.org/zap"
)

// SanitizeTimestamp converts a DateOnly timestamp to a time.Time.
func SanitizeTimestamp(date string) (time.Time, error) {
	timestamp, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return timestamp, fmt.Errorf("'start-date' flag malformed, must be in YYYY-MM-DD format: %w", err)
	}
	return timestamp, nil
}

// PrintPRList outputs the passed PR data to the console.
func PrintPRList(prMap map[string][]*github.PullRequest, format string) error {
	switch format {
	case "terminal":
		return printTerminalPRList(prMap)
	case "discord":
		return printDiscordPRList(prMap)
	default:
		return errors.New("unsupported format option %s, allowed: 'terminal', 'discord'")
	}
}

func printDiscordPRList(prMap map[string][]*github.PullRequest) error {
	zap.S().Named("output").Info("Copy paste the below into discord")
	fmt.Println()

	var keys []string
	for k := range prMap {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		fmt.Printf("**%s**:\n", k)
		prs := prMap[k]
		for _, pr := range prs {
			fmt.Printf("- #%d: [%s](<%s>)\n", pr.GetNumber(), pr.GetTitle(), pr.GetHTMLURL())
		}
		fmt.Println()
	}
	return nil
}

func printTerminalPRList(prMap map[string][]*github.PullRequest) error {
	zap.S().Named("output").Info("Pull Requests:")
	zap.S().Named("output").Info()

	var keys []string
	for k := range prMap {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		zap.S().Named("output").Infof("%s:", k)
		prs := prMap[k]
		for _, pr := range prs {
			zap.S().Named("output").Infof("#%d: %s (%s)", pr.GetNumber(), pr.GetTitle(), pr.GetHTMLURL())
		}
		zap.S().Named("output").Info()
	}
	return nil
}
