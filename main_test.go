package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	defaultProfile  = "default"
	namedProfile    = "named"
	wrongProfile    = "wrong_profile"
	configFile      = "./test/config"
	wrongConfigFile = "./test/wrong_config"
	deviceSerial    = "arn:aws:iam::123456789123:mfa/user"
	badInput        = "; rm -rf /"
)

type mockListMFADevicesAPI func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error)

func (m mockListMFADevicesAPI) ListMFADevices(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
	return m(ctx, params, optFns...)
}

func TestValidateFlags(t *testing.T) {
	var tests = []struct {
		name            string
		inputOptions    options
		inputConfigfile string
		want            error
	}{
		{"defaultProfile", options{profile: defaultProfile, code: "123456", name: namedProfile}, configFile, nil},
		{"namedProfile", options{profile: namedProfile, code: "123456", name: namedProfile}, configFile, nil},
		{"nonexistentProfile", options{profile: wrongProfile, code: "123456", name: namedProfile}, configFile, errProfileDoesNotExist},
		{"codeTooShort", options{profile: defaultProfile, code: "123", name: namedProfile}, configFile, errInvalidCode},
		{"codeTooLong", options{profile: defaultProfile, code: "1234567", name: namedProfile}, configFile, errInvalidCode},
		{"codeWithLetters", options{profile: defaultProfile, code: "12345a", name: namedProfile}, configFile, errInvalidCode},
		{"sharedConfigFileDoesNotExist", options{profile: defaultProfile, code: "123456", name: namedProfile}, wrongConfigFile, errSharedConfigUnavailable},
		{"explicitlySetDevice", options{profile: defaultProfile, code: "123456", name: namedProfile, device: deviceSerial}, configFile, nil},
		{"wrongDeviceSerial", options{profile: defaultProfile, code: "123456", name: namedProfile, device: badInput}, configFile, errInvalidDevice},
		{"wrongResultingProfileName", options{profile: defaultProfile, code: "123456", name: badInput}, configFile, errInvalidProfileName},
		{"wrongProfileName", options{profile: badInput, code: "123456", name: namedProfile}, configFile, errInvalidProfileName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validateFlags(&tt.inputOptions, tt.inputConfigfile)
			if !errors.Is(res, tt.want) {
				t.Errorf("got %q, want %q", res, tt.want)
			}
		})
	}
}

func TestGetFirstDevice(t *testing.T) {
	var tests = []struct {
		name       string
		client     func(t *testing.T) iamListMFADevicesAPI
		wantDevice string
		wantError  error
	}{
		{
			name: "oneDevice",
			client: func(t *testing.T) iamListMFADevicesAPI {
				return mockListMFADevicesAPI(func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
					t.Helper()
					return &iam.ListMFADevicesOutput{
						MFADevices: []types.MFADevice{{SerialNumber: aws.String(deviceSerial)}},
					}, nil
				})
			},
			wantDevice: deviceSerial,
			wantError:  nil,
		},
		{
			name: "manyDevices",
			client: func(t *testing.T) iamListMFADevicesAPI {
				return mockListMFADevicesAPI(func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
					t.Helper()
					return &iam.ListMFADevicesOutput{
						MFADevices: []types.MFADevice{
							{SerialNumber: aws.String(deviceSerial)},
							{SerialNumber: aws.String(deviceSerial)},
						},
					}, nil
				})
			},
			wantDevice: deviceSerial,
			wantError:  nil,
		},
		{
			name: "noDevices",
			client: func(t *testing.T) iamListMFADevicesAPI {
				return mockListMFADevicesAPI(func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
					t.Helper()
					return &iam.ListMFADevicesOutput{MFADevices: []types.MFADevice{}}, nil
				})
			},
			wantDevice: "",
			wantError:  errNoDevicesAssociated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := getFirstDevice(tt.client(t))
			if !errors.Is(err, tt.wantError) {
				t.Errorf("error %v", err)
			}
			if res != tt.wantDevice {
				t.Errorf("got %q, want %q", res, tt.wantDevice)
			}
		})
	}
}
