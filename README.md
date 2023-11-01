# mfaprof - a tool to authenticate your AWS CLI access using MFA
`mfaprof` was created for the following use case. Let's say you have a IAM user with policies attached
that only allow actions under MFA, i.e. you have the following condition in your policies:

```"Condition": {"Bool": {"aws:MultiFactorAuthPresent": "true"}}```

In this case it's not sufficient to run `aws configure` with your IAM user credentials. You have to issue STS token and authenticate with that. More on this in the [official documentation](https://aws.amazon.com/premiumsupport/knowledge-center/authenticate-mfa-cli/).

This utility allows you to quickly provision new derived AWS CLI profiles that are MFA authenticated.

### Usage
Let's say you have a AWS CLI profile for your IAM user configured. Now to create a new derived profile with MFA present you can run the following command:

```mfaprof -code <YOUR_MFA_TOKEN_CODE>```

*Tip:* The tool will ask for the code interactively if `-code` flag is not specified.

With this arguments the tool will go ahead and get the first MFA device serial number, authenticate using the provided code and credentials configured in the currently active profile and create a new profile called `<YOUR_PROFILE_NAME>_mfa` with temporary credentials. `YOUR_PROFILE_NAME` is whatever is set in `AWS_PROFILE` environment variable or "default". You can also specify the profile name explicitly with `-profile` argument. 

*Tip:* if you want to use the new profile straight away you need to set `AWS_PROFILE` environment variable. The tool will output a command to do that.

The tool supports the following arguments:
- `-code` - (Optional) The value provided by the MFA device. When omitted, an interactive prompt is shown.
- `-profile` - (Optional) Name of the AWS CLI profile to authenticate. If not specified, AWS SDK determines the value.
- `-device` - (Optional) Either the serial number for a hardware device or an ARN for a virtual device. Default: the first MFA device from ListMFADevices API call.
- `-name` - (Optional) Name of the resulting AWS CLI profile. Default: current or specified AWS CLI profile name + _mfa.
- `-debug` - (Optional) Enables debug messages, ignores quiet flag.
- `-quiet` - (Optional) Suppress non-debug messages.

Due to how Go's `flag` package works, arguments can also start with double dashes.

### Installation
1. Get the latest binary for your platform from GitHub Releases or clone the repo and run `make` to build binaries in `./bin` directory.
2. Copy the binary to a directory in your `$PATH` (e.g. to `/usr/local/bin`) and make sure it is executable.

### Security
Being a security tool, `mfaprof` doesn't use any 3rd party code besides AWS SDK. This is to mitigate the risk of vulnerabilities in dependencies.

### Limitations
The tool assumes that IAM's ListMFADevices API operation is available to the user without the MFA condition. If not, please provide MFA device serial explicitly.