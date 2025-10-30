# iam.tf - Simplified without the old OIDC provider


# IAM Role for EC2 instances with Tailscale OIDC capability
resource "aws_iam_role" "tailscale_ec2_oidc" {
  name               = "${var.role_prefix}example-ec2-role"
  assume_role_policy = data.aws_iam_policy_document.tailscale_ec2_assume_role.json
}

# Trust policy for the role
data "aws_iam_policy_document" "tailscale_ec2_assume_role" {
  # Allow EC2 service to assume this role (standard EC2 assumption)
  statement {
    effect = "Allow"
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
    actions = ["sts:AssumeRole"]
  }

  # Allow SSM to assume this role (for Session Manager)
  statement {
    effect = "Allow"
    actions = ["sts:AssumeRole"]
    sid     = "ssm"
    principals {
      type        = "Service"
      identifiers = ["ssm.amazonaws.com"]
    }
  }
}

# Basic permissions for the role
data "aws_iam_policy_document" "tailscale_ec2_permissions" {
  # Allow reading own instance metadata
  statement {
    effect = "Allow"
    actions = [
      "ec2:DescribeInstances",
      "ec2:DescribeTags"
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "ec2:SourceInstanceARN"
      values   = ["arn:aws:ec2:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:instance/*"]
    }
  }

  # Allow STS operations for token exchange
  statement {
    effect = "Allow"
    actions = [
      "sts:GetCallerIdentity",
      "sts:AssumeRoleWithWebIdentity"
    ]
    resources = ["*"]
  }

  # CloudWatch Logs
  statement {
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]
    resources = [
      "arn:aws:logs:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:log-group:/tailscale/*"
    ]
  }

  # SSM Session Manager
  statement {
    effect = "Allow"
    actions = [
      "ssmmessages:CreateDataChannel",
      "ssmmessages:OpenDataChannel",
      "ssmmessages:CreateControlChannel",
      "ssmmessages:OpenControlChannel",
      "ssm:UpdateInstanceInformation"
    ]
    resources = ["*"]
  }
}

# Attach permissions policy to the role
resource "aws_iam_role_policy" "tailscale_ec2_permissions" {
  name   = "TailscaleEC2OIDCPermissions"
  role   = aws_iam_role.tailscale_ec2_oidc.id
  policy = data.aws_iam_policy_document.tailscale_ec2_permissions.json
}

# Instance profile for EC2 instances
resource "aws_iam_instance_profile" "tailscale_ec2_oidc" {
  name = "${var.role_prefix}example-ec2-profile"
  role = aws_iam_role.tailscale_ec2_oidc.name
}

# Outputs
output "tailscale_role_arn" {
  description = "ARN of the IAM role for Tailscale OIDC"
  value       = aws_iam_role.tailscale_ec2_oidc.arn
}

output "instance_profile_name" {
  description = "Name of the instance profile to attach to EC2 instances"
  value       = aws_iam_instance_profile.tailscale_ec2_oidc.name
}

output "instance_profile_arn" {
  description = "ARN of the instance profile"
  value       = aws_iam_instance_profile.tailscale_ec2_oidc.arn
}