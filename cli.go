package main

import (
	"errors"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/serenibyss/nhprtracker/internal"
)

const cliDescription = `` // todo

var debugLogs bool

func main() {
	app := &cli.App{
		Name:        internal.AppName,
		Usage:       "CLI to gather PRs merged to a main/master branch and not a release branch",
		Description: cliDescription,
		Suggest:     true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "token",
				Aliases:     []string{"t"},
				Usage:       "set a github token to use for authenticating with github api",
				DefaultText: "'GITHUB_TOKEN' environment variable",
			},
			&cli.StringFlag{
				Name:    "start-date",
				Aliases: []string{"d"},
				Value:   internal.DefaultStartDate,
				Usage:   "set a start date to check for PRs and commits, format YYYY-MM-DD",
			},
			&cli.StringFlag{
				Name:    "organization",
				Aliases: []string{"org", "o"},
				Value:   internal.DefaultOrganization,
				Usage:   "set an organization to check for PRs and commits",
			},
			&cli.StringFlag{
				Name:    "release-branch",
				Aliases: []string{"branch", "b"},
				Value:   internal.DefaultReleaseBranch,
				Usage:   "target branch to check against when scanning master/main branch",
			},
			&cli.StringSliceFlag{
				Name:    "repos",
				Aliases: []string{"r"},
				Usage:   "select specific repos to target for PR checking",
			},
			&cli.StringFlag{
				Name:    "formatting",
				Aliases: []string{"f"},
				Value:   internal.DefaultFormatting,
				Usage:   "formatting for output text. Either 'terminal' for command line formatting, or 'discord' for copy-pasting",
			},
			&cli.BoolFlag{
				Name:   "debug",
				Hidden: true,
			},
		},
		Before: func(cCtx *cli.Context) error {
			if cCtx.Bool("debug") {
				debugLogs = true
			}
			zap.S().Debug(internal.AppVersion())
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "all-prs",
				Usage: "Gather all PRs merged into the master/main branch after the specified date",
				Action: func(cCtx *cli.Context) error {
					org := cCtx.String("organization")
					branch := cCtx.String("release-branch")
					repos := cCtx.StringSlice("repos")
					token := cCtx.String("token")
					if token != "" && !strings.HasPrefix(token, "ghp_") {
						return errors.New("provided token malformed, must use a valid GitHub token")
					}
					format := cCtx.String("formatting")
					if format != "discord" && format != "terminal" {
						format = "terminal"
					}

					timestamp, err := SanitizeTimestamp(cCtx.String("start-date"))
					if err != nil {
						return err
					}

					client, err := GetClient(org, branch, repos, timestamp, token)
					if err != nil {
						return err
					}

					// Gather repositories to check
					repoList, err := GatherRepositories(client)
					if err != nil {
						return err
					}

					// Gather all PRs merged after the specified date
					prs, err := GatherMergedPRs(client, repoList)
					if err != nil {
						if prs == nil || len(prs) == 0 {
							return err
						}
						zap.S().Error(err)
					}

					// Print out the PRs
					return PrintPRList(prs, format)
				},
			},
			{
				Name:  "unmerged-prs",
				Usage: "Gather PRs merged into the master/main branch, but not the specified release branch after the specified date",
				Action: func(cCtx *cli.Context) error {
					org := cCtx.String("organization")
					branch := cCtx.String("release-branch")
					repos := cCtx.StringSlice("repos")
					token := cCtx.String("token")
					if token != "" && !strings.HasPrefix(token, "ghp_") {
						return errors.New("provided token malformed, must use a valid GitHub token")
					}
					format := cCtx.String("formatting")
					if format != "discord" && format != "terminal" {
						format = "terminal"
					}

					timestamp, err := SanitizeTimestamp(cCtx.String("start-date"))
					if err != nil {
						return err
					}

					client, err := GetClient(org, branch, repos, timestamp, token)
					if err != nil {
						return err
					}

					// Gather repositories to check
					repoList, err := GatherRepositories(client)
					if err != nil {
						return err
					}

					// Gather repositories with release branches to compare with
					releaseRepos, err := GatherReleaseRepositories(client, repoList)
					if err != nil {
						return err
					}

					// Gather all PRs merged after the specified date
					prs, err := GatherMergedPRs(client, repoList)
					if err != nil {
						if prs == nil || len(prs) == 0 {
							return err
						}
						zap.S().Error(err)
					}

					finalPrs, err := FilterMatchingCommitsOnBranch(client, prs, releaseRepos)
					if err != nil {
						if finalPrs == nil || len(finalPrs) == 0 {
							return err
						}
						zap.S().Error(err)
					}

					return PrintPRList(finalPrs, format)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		zap.S().Error(err)
		os.Exit(1)
	}
}

func init() {
	zap.ReplaceGlobals(zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey:  "message",
			LevelKey:    "level",
			EncodeLevel: zapcore.LowercaseColorLevelEncoder,
		}),
		zapcore.Lock(os.Stderr),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return debugLogs || lvl > zapcore.DebugLevel
		}),
	)))
}
