package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mgeovany/sentra/cli/internal/storage"
)

func runStorage(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: sentra storage setup|status|test|reset")
	}

	switch args[0] {
	case "setup":
		if len(args) != 1 {
			return errors.New("usage: sentra storage setup")
		}
		return runStorageSetup()
	case "status":
		if len(args) != 1 {
			return errors.New("usage: sentra storage status")
		}
		return runStorageStatus()
	case "test":
		if len(args) != 1 {
			return errors.New("usage: sentra storage test")
		}
		return runStorageTest()
	case "reset":
		if len(args) != 1 {
			return errors.New("usage: sentra storage reset")
		}
		return runStorageReset()
	default:
		return errors.New("usage: sentra storage setup|status|test|reset")
	}
}

func runStorageStatus() error {
	fmt.Println(c(ansiBoldCyan, "Storage"))

	cfg, ok, err := storage.LoadConfig()
	if err != nil {
		return err
	}
	if ok {
		successf("✔ BYOS: active")
		fmt.Println(c(ansiDim, "Provider: ") + string(cfg.Provider))
		fmt.Println(c(ansiDim, "Bucket: ") + cfg.Bucket)
		fmt.Println(c(ansiDim, "Endpoint: ") + cfg.Endpoint)
		if strings.TrimSpace(cfg.Region) != "" {
			fmt.Println(c(ansiDim, "Region: ") + cfg.Region)
		}
		fmt.Println(c(ansiDim, "Auth: ") + string(cfg.AuthMethod))
		switch cfg.AuthMethod {
		case storage.AuthAWSProfile:
			p := strings.TrimSpace(cfg.AWSProfile)
			if p == "" {
				p = "default"
			}
			fmt.Println(c(ansiDim, "AWS profile: ") + p)
		case storage.AuthStatic:
			fmt.Println(c(ansiDim, "Secret storage: ") + string(cfg.SecretLocation))
		case storage.AuthEnvOnly:
			fmt.Println(c(ansiDim, "Secret storage: ") + "env")
		}
		return nil
	}

	_, _, enabled, err := storage.ResolveS3()
	if err != nil {
		return err
	}
	if enabled {
		successf("✔ BYOS: active (env vars)")
		infof("Hint: run `sentra storage setup` to persist config")
		return nil
	}

	warnf("⚠ BYOS: disabled")
	fmt.Println(c(ansiCyan, "Run: ") + c(ansiBoldCyan, "sentra storage setup"))
	return nil
}

func runStorageTest() error {
	_, ok, err := storage.LoadConfig()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no storage config found (run: sentra storage setup)")
	}

	cfg, client, enabled, err := storage.ResolveS3()
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("storage disabled")
	}

	sp := startSpinner("Testing storage connection...")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := storage.TestS3(ctx, client, cfg); err != nil {
		sp.StopInfo("")
		return err
	}
	sp.StopSuccess("✔ storage test OK")
	successf("✔ Connected")
	successf("✔ Upload test OK")
	successf("✔ Download test OK")
	return nil
}

func runStorageReset() error {
	if err := storage.DeleteConfig(); err != nil {
		return err
	}
	successf("✔ storage config removed")
	infof("Hint: run `sentra storage setup` to enable BYOS again")
	return nil
}

func runStorageSetup() error {
	r := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println(c(ansiBoldCyan, "Choose storage provider:"))
	providerChoice, err := promptSelect(r, []string{
		"AWS S3",
		"Cloudflare R2",
		"MinIO (self-hosted)",
		"Custom S3 endpoint",
	})
	if err != nil {
		return err
	}
	fmt.Println() // Blank line after selection

	cfg := storage.Config{Version: 1, UseSSL: true}
	cfg.ID = uuid.NewString()
	switch providerChoice {
	case 1:
		cfg.Provider = storage.ProviderAWSS3
		cfg.Endpoint = "s3.amazonaws.com"
	case 2:
		cfg.Provider = storage.ProviderR2
		// user must provide endpoint
	case 3:
		cfg.Provider = storage.ProviderMinIO
		cfg.UseSSL = false
	case 4:
		cfg.Provider = storage.ProviderCustom
	}

	bucket, err := promptLine(r, "Bucket name")
	if err != nil {
		return err
	}
	cfg.Bucket = bucket

	region, err := promptLineOptional(r, "Region (optional)")
	if err != nil {
		return err
	}
	cfg.Region = region
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	endpoint, err := promptLineOptional(r, "Endpoint (optional)")
	if err != nil {
		return err
	}
	if endpoint != "" {
		cfg.Endpoint = endpoint
	}
	// AWS requires the regional endpoint for non-us-east-1 buckets.
	if cfg.Provider == storage.ProviderAWSS3 && (endpoint == "" || strings.TrimSpace(cfg.Endpoint) == "s3.amazonaws.com") {
		if strings.TrimSpace(cfg.Region) != "" && cfg.Region != "us-east-1" {
			cfg.Endpoint = "s3." + cfg.Region + ".amazonaws.com"
		}
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return errors.New("endpoint is required for this provider")
	}

	fmt.Println(c(ansiBoldCyan, "Auth method"))
	authChoice, err := promptSelect(r, []string{
		"Use AWS profile (recommended)",
		"Access key / secret",
		"Environment variables only",
	})
	if err != nil {
		return err
	}

	switch authChoice {
	case 1:
		cfg.AuthMethod = storage.AuthAWSProfile
		profile, err := promptLineOptional(r, "AWS profile (default: default)")
		if err != nil {
			return err
		}
		if strings.TrimSpace(profile) == "" {
			profile = "default"
		}
		cfg.AWSProfile = profile
		credFile, err := promptLineOptional(r, "AWS credentials file (optional)")
		if err != nil {
			return err
		}
		cfg.AWSCredentialsFile = credFile
	case 2:
		cfg.AuthMethod = storage.AuthStatic
		ak, err := promptLine(r, "Access key id")
		if err != nil {
			return err
		}
		sk, err := promptLine(r, "Secret access key")
		if err != nil {
			return err
		}
		st, err := promptLineOptional(r, "Session token (optional)")
		if err != nil {
			return err
		}
		cfg.AccessKeyID = ak

		// Prefer keychain, fallback to file.
		cfg.SecretLocation = storage.SecretKeyring
		cfg.SecretRef = storage.SecretRefForID(cfg.ID)
		if err := storage.SaveSecret(cfg.SecretRef, sk, st); err != nil {
			fmt.Println("Warning: could not save to keychain; falling back to local file (~/.sentra/storage.json)")
			cfg.SecretLocation = storage.SecretFile
			cfg.SecretKey = sk
			cfg.SessionToken = st
			cfg.SecretRef = ""
		}
	case 3:
		cfg.AuthMethod = storage.AuthEnvOnly
		cfg.SecretLocation = storage.SecretNone
	default:
		return errors.New("invalid auth method")
	}

	test, err := promptYesNo(r, "Test connection?", true)
	if err != nil {
		return err
	}
	if test {
		s3cfg, client, _, err := storage.ResolveS3FromConfig(cfg)
		if err != nil {
			return err
		}
		infof("Testing connection...")
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := storage.TestS3(ctx, client, s3cfg); err != nil {
			return err
		}
		successf("✔ Connected")
		successf("✔ Upload test OK")
		successf("✔ Download test OK")
	}

	if err := storage.SaveConfig(cfg); err != nil {
		return err
	}
	p, _ := storage.DefaultPath()
	fmt.Printf("Saved to %s\n", p)
	return nil
}

func promptLine(r *bufio.Reader, label string) (string, error) {
	for {
		v, err := promptBox(r, label, "", "")
		if err != nil {
			fmt.Println(c(ansiYellow, "Value required"))
			continue
		}
		// Ensure value is not empty even in non-TTY mode
		v = strings.TrimSpace(v)
		if v == "" {
			fmt.Println(c(ansiYellow, "Value required"))
			continue
		}
		return v, nil
	}
}

func promptLineOptional(r *bufio.Reader, label string) (string, error) {
	// Don't enforce required; allow blank.
	if !isTTY(os.Stdout) {
		fmt.Printf("%s: ", label)
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}

	label = strings.TrimSpace(label)
	inner := 66
	border := "+" + strings.Repeat("-", inner) + "+"
	inputLinePrefix := "| "
	inputLineSuffix := " |"
	contentWidth := inner - 2

	fmt.Println(c(ansiBoldCyan, label))
	fmt.Println(c(ansiDim, border))
	fmt.Print(c(ansiDim, inputLinePrefix))

	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(line)

	shown := v
	if len(shown) > contentWidth {
		if contentWidth >= 3 {
			shown = shown[:contentWidth-3] + "..."
		} else {
			shown = shown[:contentWidth]
		}
	}
	pad := ""
	if n := contentWidth - len(shown); n > 0 {
		pad = strings.Repeat(" ", n)
	}

	_, _ = fmt.Fprint(os.Stdout, "\x1b[1A")
	_, _ = fmt.Fprint(os.Stdout, "\r\x1b[2K")
	_, _ = fmt.Fprint(os.Stdout, c(ansiDim, inputLinePrefix)+shown+pad+c(ansiDim, inputLineSuffix))
	_, _ = fmt.Fprint(os.Stdout, "\x1b[1B")
	fmt.Println(c(ansiDim, border))

	return v, nil
}

func promptYesNo(r *bufio.Reader, label string, def bool) (bool, error) {
	if def {
		fmt.Printf("%s (Y/n) ", label)
	} else {
		fmt.Printf("%s (y/N) ", label)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return def, nil
	}
	switch line {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return def, nil
	}
}
