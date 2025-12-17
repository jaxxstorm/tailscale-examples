# AWS Tailscale OIDC Integration

This Terraform configuration sets up an EC2 instance that authenticates to Tailscale using AWS OIDC tokens instead of OAuth client secrets.

## Prerequisites

1. An AWS account with appropriate permissions
2. A Tailscale account with admin access
3. An SSH key pair already created in AWS
4. Terraform installed

## Setup Instructions

### 1. Configure Tailscale OIDC Trust Credential

Before running Terraform, you need to configure the OIDC trust credential in Tailscale:

1. Go to the [Tailscale Admin Console](https://login.tailscale.com/admin)
2. Navigate to **Settings** → **Trust credentials**
3. Click **+ Trust Credential** button
4. Select **OpenID Connect**
5. Fill in the following fields:

   - **Description**: `AWS Dev` (or any descriptive name)
   
   - **Issuer**: Select `Custom issuer`
   
   - **Issuer URL**: `https://<YOUR_STS_ENDPOINT>.tokens.sts.global.api.aws`
     - Replace `<YOUR_STS_ENDPOINT>` with your AWS STS regional endpoint ID
     - Example: `https://a153619b-1bb2-44f1-8ebb-282d26a9d9e5.tokens.sts.global.api.aws`
     - You can find this by running the Terraform first (it will fail) or by checking AWS STS documentation
   
   - **Subject**: `arn:aws:iam::<AWS_ACCOUNT_ID>:role/tailscale-oidc-*`
     - Replace `<AWS_ACCOUNT_ID>` with your AWS account ID
     - The `*` wildcard allows any role starting with `tailscale-oidc-`
     - Or use the exact ARN output from `terraform output tailscale_subject` after first apply
   
   - **Audience** (optional): `api.tailscale.com/<CLIENT_ID>`
     - This will be shown after you create the credential
     - Copy this value for your `terraform.tfvars`
   
   - **Client ID**: This will be automatically generated
     - Copy this value for your `terraform.tfvars`

6. Click **Create**

### 2. Configure Terraform Variables

Create or update `terraform.tfvars` with your values:

```hcl
key_pair_name = "your-aws-key-pair-name"
tailscale_client_id = "TdWjeTt8mN11CNTRL-kipyxVChL621CNTRL"  # From Tailscale OIDC setup
tailscale_audience = "api.tailscale.com/TdWjeTt8mN11CNTRL-kipyxVChL621CNTRL"  # From Tailscale OIDC setup
advertise_tags = ["tag:subnet-router"]  # Tailscale tags for ACLs
```

### 3. Deploy

```bash
terraform init
terraform plan
terraform apply
```

### 4. Verify

After deployment, the EC2 instance should:
- Automatically authenticate to Tailscale using AWS OIDC tokens
- Appear in your Tailscale admin console
- Advertise the VPC CIDR as a subnet route (if configured)

## How It Works

1. The EC2 instance has an IAM role with permission to call `sts:GetWebIdentityToken`
2. On boot, the instance requests a web identity token from AWS STS with the Tailscale audience
3. This token is used to authenticate to Tailscale instead of an OAuth client secret
4. The token is short-lived (300 seconds) and automatically refreshed

## Outputs

- `tailscale_subject`: The ARN of the IAM role that will be used in the OIDC subject claim

## Security Notes

- The instance uses IMDSv2 (Instance Metadata Service v2) for enhanced security
- Tokens are limited to 300-second duration
- The IAM role is scoped to only allow token generation with the specific Tailscale audience
