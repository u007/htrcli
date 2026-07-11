package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htrcli/internal/output"
)

var (
	publishSourceDir   string
	publishChannel     string
	publishAPIKey      string
	publishAPISecret   string
	publishBuild       bool
	publishWebExt      string
	publishDryRun      bool
	publishSignTimeout int
)

// channelLabel returns a human-readable description of an AMO channel.
// "listed"   = public on addons.mozilla.org (anyone can install).
// "unlisted" = self-distributed / "own use" (only the developer installs it).
func channelLabel(channel string) string {
	if channel == "listed" {
		return "listed (public on addons.mozilla.org)"
	}
	return "unlisted (self-distributed / own use)"
}

// firstEnv returns the first non-empty value among the given environment keys.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// resolveWebExt returns the command + leading args to invoke web-ext.
// If `web-ext` is on PATH we use it directly; otherwise we fall back to
// `npx --yes web-ext` so it is fetched on demand.
func resolveWebExt() (string, []string) {
	if _, err := exec.LookPath("web-ext"); err == nil {
		return "web-ext", nil
	}
	return "npx", []string{"--yes", "web-ext"}
}

// runCmd runs a command, streaming its output to the terminal.
func runCmd(name string, args []string, dir string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Build and submit the Firefox add-on to addons.mozilla.org (AMO)",
	Long: `Build (optionally) and sign the Firefox add-on, then submit it to
addons.mozilla.org (AMO) via web-ext.

Channels:
  --channel=listed    Public listing on addons.mozilla.org (default).
  --channel=unlisted  Self-distributed ("own use") — not shown in the gallery.

Credentials (AMO API key + secret) are resolved in this order:
  1. --api-key / --api-secret flags
  2. Environment: AMO_API_KEY / AMO_API_SECRET (or HTRCLI_AMO_API_KEY / HTRCLI_AMO_API_SECRET)
  3. htrcli config:  htrcli config set amo-api-key / amo-api-secret

Get credentials at:
  https://addons.mozilla.org/en-US/developers/addon/api/key/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve channel (default: listed = public).
		channel := publishChannel
		if channel == "" {
			channel = "listed"
		}
		if channel != "listed" && channel != "unlisted" {
			return fmt.Errorf("invalid --channel %q (must be 'listed' or 'unlisted')", channel)
		}

		// Resolve AMO credentials.
		key := publishAPIKey
		if key == "" {
			key = firstEnv("AMO_API_KEY", "HTRCLI_AMO_API_KEY")
		}
		if key == "" {
			key = viper.GetString("amo-api-key")
		}
		secret := publishAPISecret
		if secret == "" {
			secret = firstEnv("AMO_API_SECRET", "HTRCLI_AMO_API_SECRET")
		}
		if secret == "" {
			secret = viper.GetString("amo-api-secret")
		}
		if key == "" || secret == "" {
			return fmt.Errorf(
				"AMO API credentials required: pass --api-key/--api-secret, set " +
					"AMO_API_KEY/AMO_API_SECRET, or run `htrcli config set amo-api-key/amo-api-secret`",
			)
		}

		// Resolve source directory.
		srcDir := publishSourceDir
		if srcDir == "" {
			srcDir = "firefox/build"
		}

		// Optionally build first.
		if publishBuild {
			fmt.Printf("%s Building Firefox add-on...\n", output.Info("build"))
			if err := runCmd("bun", []string{"run", "firefox:build"}, ""); err != nil {
				return fmt.Errorf("firefox build failed: %w", err)
			}
		}

		if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
			return fmt.Errorf(
				"source dir %q not found (run with --build or `bun run firefox:build` first)",
				srcDir,
			)
		}

		// Resolve the web-ext executable.
		weCmd, weArgs := resolveWebExt()
		if publishWebExt != "" {
			weCmd = publishWebExt
			weArgs = nil
		}

		artifactsDir := "web-ext-artifacts"
		signArgs := append([]string{}, weArgs...)
		signArgs = append(signArgs,
			"sign",
			"--source-dir", srcDir,
			"--channel", channel,
			"--api-key", key,
			"--api-secret", secret,
			"--artifacts-dir", artifactsDir,
		)
		if publishSignTimeout > 0 {
			signArgs = append(signArgs, "--timeout", fmt.Sprintf("%d", publishSignTimeout))
		}

		if publishDryRun {
			fmt.Printf("%s %s\n", weCmd, strings.Join(signArgs, " "))
			return nil
		}

		fmt.Printf("%s Submitting to AMO (%s)...\n", output.Info("AMO"), channelLabel(channel))
		if err := runCmd(weCmd, signArgs, ""); err != nil {
			return fmt.Errorf("web-ext sign failed: %w", err)
		}

		fmt.Printf(
			"%s Signed add-on written to %s/ — channel: %s\n",
			output.Success("✓"),
			artifactsDir,
			channelLabel(channel),
		)
		return nil
	},
}

func init() {
	publishCmd.Flags().StringVar(&publishSourceDir, "source-dir", "firefox/build",
		"path to the built (unpacked) add-on directory")
	publishCmd.Flags().StringVar(&publishChannel, "channel", "listed",
		"AMO channel: 'listed' (public) or 'unlisted' (own use / self-distributed)")
	publishCmd.Flags().StringVar(&publishAPIKey, "api-key", "", "AMO API key")
	publishCmd.Flags().StringVar(&publishAPISecret, "api-secret", "", "AMO API secret")
	publishCmd.Flags().BoolVar(&publishBuild, "build", false,
		"run `bun run firefox:build` before signing")
	publishCmd.Flags().StringVar(&publishWebExt, "web-ext", "",
		"path to the web-ext executable (default: web-ext on PATH, else npx)")
	publishCmd.Flags().BoolVar(&publishDryRun, "dry-run", false,
		"print the web-ext command instead of running it")
	publishCmd.Flags().IntVar(&publishSignTimeout, "sign-timeout", 0,
		"web-ext sign timeout in seconds (0 = web-ext default)")

	rootCmd.AddCommand(publishCmd)
}
