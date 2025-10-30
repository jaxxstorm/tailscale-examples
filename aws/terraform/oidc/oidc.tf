# oidc.tf


# Generate RSA key pair for JWT signing
resource "tls_private_key" "oidc_signing_key" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

# Store private key in Secrets Manager
resource "aws_secretsmanager_secret" "oidc_private_key" {
  name        = "oidc-provider/private-key"
  description = "Private key for OIDC JWT signing"
}

resource "aws_secretsmanager_secret_version" "oidc_private_key" {
  secret_id     = aws_secretsmanager_secret.oidc_private_key.id
  secret_string = jsonencode({
    private_key = tls_private_key.oidc_signing_key.private_key_pem
    public_key  = tls_private_key.oidc_signing_key.public_key_pem
    key_id      = "oidc-key-1"
  })
}

# Create Lambda deployment package from pre-built binary
data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/lambda/bootstrap"
  output_path = "${path.module}/lambda-function.zip"
}

# Lambda execution role
resource "aws_iam_role" "oidc_lambda_role" {
  name = "oidc-provider-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

# Lambda permissions
resource "aws_iam_role_policy" "oidc_lambda_policy" {
  name = "oidc-provider-lambda-policy"
  role = aws_iam_role.oidc_lambda_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:*"
      },
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue"
        ]
        Resource = aws_secretsmanager_secret.oidc_private_key.arn
      }
    ]
  })
}

# API Gateway (need to create this first to get the endpoint for Lambda env var)
resource "aws_apigatewayv2_api" "oidc_api" {
  name          = "oidc-provider"
  protocol_type = "HTTP"
  description   = "OIDC Provider for AWS EC2 instances"

  cors_configuration {
    allow_credentials = false
    allow_headers     = ["*"]
    allow_methods     = ["GET", "POST", "OPTIONS"]
    allow_origins     = ["*"]
    max_age          = 86400
  }
}

# Lambda function
resource "aws_lambda_function" "oidc_provider" {
  filename         = data.archive_file.lambda_zip.output_path
  function_name    = "oidc-provider"
  role            = aws_iam_role.oidc_lambda_role.arn
  handler         = "bootstrap"
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
  runtime         = "provided.al2023"
  timeout         = 30

  environment {
    variables = {
      SECRET_ARN       = aws_secretsmanager_secret.oidc_private_key.arn
      ISSUER_URL       = aws_apigatewayv2_api.oidc_api.api_endpoint
      ROLE_PREFIX      = var.role_prefix
      OIDC_AWS_REGION  = data.aws_region.current.name
      OIDC_AWS_ACCOUNT = data.aws_caller_identity.current.account_id
      KEY_ID           = "oidc-key-1"
      OIDC_TAGS     = var.oidc_tags
    }
  }
}

# API Gateway stage
resource "aws_apigatewayv2_stage" "oidc_stage" {
  api_id      = aws_apigatewayv2_api.oidc_api.id
  name        = "$default"
  auto_deploy = true

  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.oidc_api_logs.arn
    format = jsonencode({
      requestId      = "$context.requestId"
      ip            = "$context.identity.sourceIp"
      requestTime   = "$context.requestTime"
      httpMethod    = "$context.httpMethod"
      routeKey      = "$context.routeKey"
      status        = "$context.status"
      protocol      = "$context.protocol"
      responseLength = "$context.responseLength"
    })
  }
}

# Lambda permission for API Gateway
resource "aws_lambda_permission" "oidc_api_gateway" {
  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.oidc_provider.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.oidc_api.execution_arn}/*/*"
}

# Lambda integration
resource "aws_apigatewayv2_integration" "oidc_lambda_integration" {
  api_id             = aws_apigatewayv2_api.oidc_api.id
  integration_type   = "AWS_PROXY"
  integration_method = "POST"
  integration_uri    = aws_lambda_function.oidc_provider.invoke_arn
}

# Routes
resource "aws_apigatewayv2_route" "oidc_discovery" {
  api_id    = aws_apigatewayv2_api.oidc_api.id
  route_key = "GET /.well-known/openid_configuration"
  target    = "integrations/${aws_apigatewayv2_integration.oidc_lambda_integration.id}"
}

resource "aws_apigatewayv2_route" "oidc_discovery_dash" {
  api_id    = aws_apigatewayv2_api.oidc_api.id
  route_key = "GET /.well-known/openid-configuration"
  target    = "integrations/${aws_apigatewayv2_integration.oidc_lambda_integration.id}"
}

resource "aws_apigatewayv2_route" "oidc_jwks" {
  api_id    = aws_apigatewayv2_api.oidc_api.id
  route_key = "GET /.well-known/jwks.json"
  target    = "integrations/${aws_apigatewayv2_integration.oidc_lambda_integration.id}"
}

resource "aws_apigatewayv2_route" "oidc_token" {
  api_id    = aws_apigatewayv2_api.oidc_api.id
  route_key = "POST /token"
  target    = "integrations/${aws_apigatewayv2_integration.oidc_lambda_integration.id}"
}

resource "aws_apigatewayv2_route" "oidc_options" {
  api_id    = aws_apigatewayv2_api.oidc_api.id
  route_key = "OPTIONS /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.oidc_lambda_integration.id}"
}

# CloudWatch logs
resource "aws_cloudwatch_log_group" "oidc_api_logs" {
  name              = "/aws/apigateway/oidc-provider"
  retention_in_days = 7
}

resource "aws_cloudwatch_log_group" "oidc_lambda_logs" {
  name              = "/aws/lambda/oidc-provider"
  retention_in_days = 7
}

# Outputs - these replace your old oidc.tf outputs
output "oidc_issuer_url" {
  description = "OIDC Issuer URL to use in Tailscale (autogenerated API Gateway URL)"
  value       = aws_apigatewayv2_api.oidc_api.api_endpoint
}

output "oidc_discovery_url" {
  description = "OIDC Discovery URL"
  value       = "${aws_apigatewayv2_api.oidc_api.api_endpoint}/.well-known/openid_configuration"
}

output "oidc_token_endpoint" {
  description = "Token endpoint for EC2 instances"
  value       = "${aws_apigatewayv2_api.oidc_api.api_endpoint}/token"
}

output "tailscale_config" {
  description = "Configuration values for Tailscale OIDC setup"
  value = {
    issuer_url      = aws_apigatewayv2_api.oidc_api.api_endpoint
    audience        = var.tailscale_audience
    subject_pattern = "system:role:${data.aws_caller_identity.current.account_id}:${var.role_prefix}*"
    role_prefix     = var.role_prefix
    account_id      = data.aws_caller_identity.current.account_id
    region          = data.aws_region.current.name
  }
}