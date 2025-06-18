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
    sid    = "EniAttachAndDisableSrcDst"
    effect = "Allow"
    actions = [
      "ec2:AttachNetworkInterface",
      "ec2:ModifyInstanceAttribute",
      "ec2:DescribeInstances",
      "ec2:DescribeNetworkInterfaces",
    ]

    resources = [
      "arn:aws:ec2:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:instance/*",
      aws_network_interface.main.arn
    ]
  }
}

resource "aws_iam_instance_profile" "main" {
  name = var.name
  role = aws_iam_role.main.name

  tags = var.tags
}

resource "aws_iam_role" "main" {
  name = var.name

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
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ssm.amazonaws.com"
        }
      }
    ]
  })

  tags = var.tags
}

resource "aws_iam_role_policy" "main" {
  name   = var.name
  role   = aws_iam_role.main.id
  policy = data.aws_iam_policy_document.main.json

  depends_on = [aws_iam_instance_profile.main]
}
