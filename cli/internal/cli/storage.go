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
	cfg, ok, err := storage.LoadConfig()
	if err != nil {
		return err
	}
	if ok {
		fmt.Println("BYOS: active")
		fmt.Printf("Provider: %s\n", cfg.Provider)
		fmt.Printf("Bucket: %s\n", cfg.Bucket)
		fmt.Printf("Endpoint: %s\n", cfg.Endpoint)
		if strings.TrimSpace(cfg.Region) != "" {
			fmt.Printf("Region: %s\n", cfg.Region)
		}
		fmt.Printf("Auth: %s\n", cfg.AuthMethod)
		switch cfg.AuthMethod {
		case storage.AuthAWSProfile:
			p := strings.TrimSpace(cfg.AWSProfile)
			if p == "" {
				p = "default"
			}
			fmt.Printf("AWS profile: %s\n", p)
		case storage.AuthStatic:
			fmt.Printf("Secret storage: %s\n", cfg.SecretLocation)
		case storage.AuthEnvOnly:
			fmt.Println("Secret storage: env")
		}
		return nil
	}

	_, _, enabled, err := storage.ResolveS3()
	if err != nil {
		return err
	}
	if enabled {
		fmt.Println("BYOS: active (env vars)")
		fmt.Println("Hint: run `sentra storage setup` to persist config")
		return nil
	}

	fmt.Println("BYOS: disabled")
	fmt.Println("Run: sentra storage setup")
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

	fmt.Println("Testing connection...")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := storage.TestS3(ctx, client, cfg); err != nil {
		return err
	}
	fmt.Println("✔ Connected")
	fmt.Println("✔ Upload test OK")
	fmt.Println("✔ Download test OK")
	return nil
}

func runStorageReset() error {
	if err := storage.DeleteConfig(); err != nil {
		return err
	}
	fmt.Println("✔ storage config removed")
	return nil
}

func runStorageSetup() error {
	r := bufio.NewReader(os.Stdin)

	fmt.Println("Choose storage provider:")
	providerChoice, err := promptSelect(r, []string{
		"AWS S3",
		"Cloudflare R2",
		"MinIO (self-hosted)",
		"Custom S3 endpoint",
	})
	if err != nil {
		return err
	}

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

	fmt.Println("Auth method:")
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
		fmt.Println("Testing connection...")
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := storage.TestS3(ctx, client, s3cfg); err != nil {
			return err
		}
		fmt.Println("✔ Connected")
		fmt.Println("✔ Upload test OK")
		fmt.Println("✔ Download test OK")
	}

	if err := storage.SaveConfig(cfg); err != nil {
		return err
	}
	p, _ := storage.DefaultPath()
	fmt.Printf("Saved to %s\n", p)
	return nil
}

func promptSelect(r *bufio.Reader, options []string) (int, error) {
	for i, o := range options {
		fmt.Printf("%d) %s\n", i+1, o)
	}
	for {
		fmt.Print("> ")
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		n := 0
		for _, ch := range line {
			if ch < '0' || ch > '9' {
				n = 0
				break
			}
			n = n*10 + int(ch-'0')
		}
		if n >= 1 && n <= len(options) {
			return n, nil
		}
		fmt.Println("Invalid choice")
	}
}

func promptLine(r *bufio.Reader, label string) (string, error) {
	for {
		fmt.Printf("%s: ", label)
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
		fmt.Println("Value required")
	}
}

func promptLineOptional(r *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
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
