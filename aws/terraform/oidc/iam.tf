data "aws_region" "current" {}

data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "main" {

  dynamic "statement" {
    for_each = var.enable_aws_ssm ? ["x"] : []

    content {
      sid    = "SessionManager"
      effect = "Allow"
      actions = [
        "ssmmessages:CreateDataChannel",
        "ssmmessages:OpenDataChannel",
        "ssmmessages:CreateControlChannel",
        "ssmmessages:OpenControlChannel",
        "ssm:UpdateInstanceInformation",
      ]
      resources = [
        #checkov:skip=CKV_AWS_111:FIXME - need to restrict this
        "*"
      ]
    }

  }

  statement {
    sid     = "AllowGetWebIdentityTokenForTailscale"
    effect  = "Allow"
    actions = ["sts:GetWebIdentityToken"]
    resources = ["*"]

    condition {
      test     = "ForAnyValue:StringEquals"
      variable = "sts:IdentityTokenAudience"
      values   = [var.tailscale_audience]
    }

    condition {
      test     = "NumericLessThanEquals"
      variable = "sts:DurationSeconds"
      values   = ["300"]
    }
  }
}

resource "aws_iam_instance_profile" "main" {
  name = var.hostname
  role = aws_iam_role.main.name

}

resource "aws_iam_role" "main" {
  name = var.hostname

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })

}

resource "aws_iam_role_policy_attachment" "ssm_core" {
  count      = var.enable_aws_ssm ? 1 : 0
  role       = aws_iam_role.main.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_role_policy" "main" {
  name   = var.hostname
  role   = aws_iam_role.main.id
  policy = data.aws_iam_policy_document.main.json

  depends_on = [aws_iam_instance_profile.main]
}

output "tailscale_subject" {
  value = aws_iam_role.main.arn
}