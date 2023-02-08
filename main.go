package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	defaultNameSuffix        = "_mfa"
	awsMfaCodePattern        = `^\d{6,6}$`
	awsMfaDevicePattern      = `^[\w+=/:,.@-]{9,256}$`
	awsCliProfileNamePattern = `^[\w-]+$`
	awsProfileSectionPattern = `\[profile %s\]`
	awsDefaultProfile        = "default"
)

var (
	errInvalidCode             = errors.New("code must be exactly 6 digits")
	errInvalidDevice           = errors.New("device serial number is not valid")
	errInvalidProfileName      = errors.New("profile name must be alphanumeric (underscores and hyphens allowed)")
	errSharedConfigUnavailable = errors.New("cannot access shared config")
	errCannotParseConfig       = errors.New("cannot parse config file")
	errProfileDoesNotExist     = errors.New("profile does not exist")
	errNoDevicesAssociated     = errors.New("there are no MFA devices associated with this user")
	errCannotListDevices       = errors.New("cannot list MFA devices")
)

type options struct {
	profile, name, code, device, region string
	debug                               bool
}

type iamListMFADevicesAPI interface {
	ListMFADevices(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error)
}

func parseFlags() *options {
	opt := options{}
	flag.StringVar(&opt.profile, "profile", "",
		"(Optional) Name of the AWS CLI profile to authenticate. If not specified, AWS SDK determines the value.")
	flag.StringVar(&opt.name, "name", "",
		"(Optional) Name of the resulting AWS CLI profile. Default: current or specified AWS CLI profile name + _mfa.")
	flag.StringVar(&opt.code, "code", "",
		"(Madnatory) The value provided by the MFA device.")
	flag.StringVar(&opt.device, "device", "",
		"(Optional) Either the serial number for a hardware device or an ARN for a virtual device. "+
			"Default: the first MFA device from ListMFADevices API call.")
	flag.BoolVar(&opt.debug, "debug", false, "(Optional) Enables debug messages.")
	flag.Parse()
	return &opt
}

func validateFlags(opt *options, configFilename string) error {
	codeInputValid, _ := regexp.MatchString(awsMfaCodePattern, opt.code)
	if !codeInputValid {
		return errInvalidCode
	}

	if opt.device != "" {
		serialInputValid, _ := regexp.MatchString(awsMfaDevicePattern, opt.device)
		if !serialInputValid {
			return errInvalidDevice
		}
	}

	awsCliProfileNameRegexp := regexp.MustCompile(awsCliProfileNamePattern)
	if nameInputValid := awsCliProfileNameRegexp.MatchString(opt.name); !nameInputValid {
		return errInvalidProfileName
	}
	if profileInputValid := awsCliProfileNameRegexp.MatchString(opt.profile); !profileInputValid {
		return errInvalidProfileName
	}

	cfgFile, err := os.ReadFile(configFilename)
	if err != nil {
		return fmt.Errorf("%w: %v", errSharedConfigUnavailable, err)
	}
	profileExists, err := regexp.Match(fmt.Sprintf(awsProfileSectionPattern, opt.profile), cfgFile)
	if err != nil {
		return fmt.Errorf("%w: %v", errCannotParseConfig, err)
	}
	if !profileExists {
		return errProfileDoesNotExist
	}

	return nil
}

func getFirstDevice(api iamListMFADevicesAPI) (string, error) {
	mfaDevices, err := api.ListMFADevices(context.TODO(), &iam.ListMFADevicesInput{})
	if err != nil {
		return "", fmt.Errorf("%w: %v", errCannotListDevices, err)
	}
	if len(mfaDevices.MFADevices) == 0 {
		return "", errNoDevicesAssociated
	}
	return *mfaDevices.MFADevices[0].SerialNumber, nil
}

func saveNewProfile(name string, region string, stsOutput *sts.GetSessionTokenOutput) error {
	// cmd.Run() doesn't invoke shell and doesn't evaluate globs
	log.Printf("Running command 1 out of 4: aws configure set aws_access_key_id <VALUE> --profile %s", name)
	err := exec.Command(
		"aws", "configure", "set", "aws_access_key_id",
		*stsOutput.Credentials.AccessKeyId, "--profile", name).Run()
	if err != nil {
		return err
	}
	log.Printf("Running command 2 out of 4: aws configure set aws_secret_access_key <VALUE> --profile %s", name)
	err = exec.Command(
		"aws", "configure", "set", "aws_secret_access_key",
		*stsOutput.Credentials.SecretAccessKey, "--profile", name).Run()
	if err != nil {
		return err
	}
	log.Printf("Running command 3 out of 4: aws configure set aws_session_token <VALUE> --profile %s", name)
	err = exec.Command(
		"aws", "configure", "set", "aws_session_token",
		*stsOutput.Credentials.SessionToken, "--profile", name).Run()
	if err != nil {
		return err
	}
	log.Printf("Running command 4 out of 4: aws configure set region %s --profile %s", region, name)
	err = exec.Command(
		"aws", "configure", "set", "region",
		region, "--profile", name).Run()
	if err != nil {
		return err
	}
	return nil
}

func main() {
	opt := parseFlags()

	debug := func(format string, a ...any) {
		if opt.debug {
			log.Printf("[DEBUG] "+format, a...)
		}
	}

	// We need to resolve shared config filename to validate if the provided profile exists
	configFile := config.DefaultSharedConfigFilename()
	envConfig, err := config.NewEnvConfig()
	if err != nil {
		log.Fatal(err)
	}
	if envConfig.SharedConfigFile != "" {
		configFile = envConfig.SharedConfigFile
	}
	debug("Using shared config file %q", configFile)
	if opt.profile == "" {
		if envConfig.SharedConfigProfile != "" {
			opt.profile = envConfig.SharedConfigProfile
		} else {
			opt.profile = awsDefaultProfile
		}
	}

	if opt.name == "" {
		opt.name = opt.profile + defaultNameSuffix
	}

	debug("Args: %+v", opt)

	err = validateFlags(opt, configFile)
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithSharedConfigProfile(opt.profile), // value is ignored when empty
	)
	if err != nil {
		log.Fatal(err)
	}

	opt.region = cfg.Region
	debug("Detected region: %s", opt.region)

	if opt.device == "" {
		log.Print("No MFA device serial number provided, getting one from ListMFADevices")
		opt.device, err = getFirstDevice(iam.NewFromConfig(cfg))
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Print("Getting temporary credentials")
	stsOutput, err := sts.NewFromConfig(cfg).GetSessionToken(
		context.TODO(),
		&sts.GetSessionTokenInput{SerialNumber: &opt.device, TokenCode: &opt.code},
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Saving new profile")
	err = saveNewProfile(opt.name, opt.region, stsOutput)
	if err != nil {
		log.Fatal(err)
	}

	message := `
The named profile "%[1]s" has been configured.
To use it set an environment variable like this.

For Linux and macOS:
	export AWS_PROFILE=%[1]s
For Windows:
	setx AWS_PROFILE %[1]s

Or use --profile argument with AWS CLI.
`
	fmt.Printf(message, opt.name)
}
