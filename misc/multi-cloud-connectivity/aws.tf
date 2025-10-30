locals {
  vpc_cidr            = "172.16.0.0/16"
  vpc_private_subnets = ["172.16.0.0/24", "172.16.1.0/24", "172.16.2.0/24"]
  vpc_public_subnets  = ["172.16.3.0/24", "172.16.4.0/24", "172.16.5.0/24"]
}

module "vpc" {

  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-perf-west"
  cidr               = local.vpc_cidr
  enable_nat_gateway = true

  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_private_subnets
  public_subnets  = local.vpc_public_subnets

}

data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

data "aws_ami" "ubuntu_2404" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

module "amz-tailscale-client-ubuntu" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = "aws"
  max_retries = 10
  retry_delay = 10
}


resource "aws_security_group" "main" {
  name        = "lbr-aws"
  description = "Used in lbr-aws instance ${module.vpc.private_subnets[0]}"
  vpc_id      = module.vpc.vpc_id


  ingress {
    description = "Allow all VPC traffic"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["172.16.0.0/16"]
  }

  egress {
    description      = "Unrestricted egress"
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = merge(var.tags, {
    Name = "lbr-aws"
  })
}



resource "aws_instance" "ubuntu" {

  count         = 1
  ami           = data.aws_ami.ubuntu_2404.id
  instance_type = "t3.large"


  iam_instance_profile = aws_iam_instance_profile.main.name

  monitoring = true

  subnet_id = module.vpc.private_subnets[0]

  user_data_base64            = module.amz-tailscale-client-ubuntu.rendered
  user_data_replace_on_change = true

  vpc_security_group_ids = [ aws_security_group.main.id ]

  tags = merge(
    var.tags,
    { "Name" = "lbr-aws" }
  )
}


output "instance_id" {
  value = aws_instance.ubuntu[*].id
}

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
      resources = ["*"
      ]
    }

  }

}

resource "aws_iam_instance_profile" "main" {
  name = "lbr-aws"
  role = aws_iam_role.main.name

  tags = var.tags
}

resource "aws_iam_role" "main" {
  name = "lbr-aws"

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
  name   = "lbr-aws"
  role   = aws_iam_role.main.id
  policy = data.aws_iam_policy_document.main.json

  depends_on = [aws_iam_instance_profile.main]
}

# ECS Cluster
resource "aws_ecs_cluster" "main" {
  name = "tailscale-demo"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = var.tags
}

# ECS Task Execution Role
resource "aws_iam_role" "ecs_task_execution_role" {
  name = "ecs-task-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })

  tags = var.tags
}

# resource "aws_iam_role_policy_attachment" "ecs_task_execution_role_policy" {
#   role       = aws_iam_role.ecs_task_execution_role.name
#   policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
# }

# # ECS Task Role (for application permissions)
# resource "aws_iam_role" "ecs_task_role" {
#   name = "ecs-task-role"

#   assume_role_policy = jsonencode({
#     Version = "2012-10-17"
#     Statement = [
#       {
#         Action = "sts:AssumeRole"
#         Effect = "Allow"
#         Principal = {
#           Service = "ecs-tasks.amazonaws.com"
#         }
#       }
#     ]
#   })

#   tags = var.tags
# }

# # CloudWatch Log Group for ECS
# resource "aws_cloudwatch_log_group" "ecs_logs" {
#   name              = "/ecs/tailscale-demo-streamer"
#   retention_in_days = 7

#   tags = var.tags
# }

# # ECS Task Definition
# resource "aws_ecs_task_definition" "tailscale_demo_streamer" {
#   family                   = "tailscale-demo-streamer"
#   requires_compatibilities = ["FARGATE"]
#   network_mode             = "awsvpc"
#   cpu                      = "256"
#   memory                   = "512"
#   execution_role_arn       = aws_iam_role.ecs_task_execution_role.arn
#   task_role_arn            = aws_iam_role.ecs_task_role.arn

#   runtime_platform {
#     cpu_architecture        = "ARM64"
#     operating_system_family = "LINUX"
#   }

#   container_definitions = jsonencode([
#     {
#       name  = "tailscale-demo-streamer"
#       image = "ghcr.io/jaxxstorm/tailscale-demo-streamer:latest"
      
#       environment = [
#         {
#           name  = "TSNET"
#           value = "true"
#         },
#         {
#           name  = "HOSTNAME"
#           value = "demo"
#         },
#         {
#           name  = "TAILSCALE_AUTHKEY"
#           value = var.tailscale_auth_key
#         }
#       ]

#       logConfiguration = {
#         logDriver = "awslogs"
#         options = {
#           "awslogs-group"         = aws_cloudwatch_log_group.ecs_logs.name
#           "awslogs-region"        = data.aws_region.current.name
#           "awslogs-stream-prefix" = "ecs"
#         }
#       }

#       essential = true
#     }
#   ])

#   tags = var.tags
# }

# # Security Group for ECS Service
# resource "aws_security_group" "ecs_service" {
#   name        = "ecs-tailscale-demo-streamer"
#   description = "Security group for ECS Tailscale demo streamer service"
#   vpc_id      = module.vpc.vpc_id

#   ingress {
#     description = "Allow all VPC traffic"
#     from_port   = 0
#     to_port     = 0
#     protocol    = "-1"
#     cidr_blocks = [local.vpc_cidr]
#   }

#   egress {
#     description      = "Unrestricted egress"
#     from_port        = 0
#     to_port          = 0
#     protocol         = "-1"
#     cidr_blocks      = ["0.0.0.0/0"]
#     ipv6_cidr_blocks = ["::/0"]
#   }

#   tags = merge(var.tags, {
#     Name = "ecs-tailscale-demo-streamer"
#   })
# }

# # ECS Service
# resource "aws_ecs_service" "tailscale_demo_streamer" {
#   name            = "tailscale-demo-streamer"
#   cluster         = aws_ecs_cluster.main.id
#   task_definition = aws_ecs_task_definition.tailscale_demo_streamer.arn
#   desired_count   = 1
#   launch_type     = "FARGATE"

#   network_configuration {
#     subnets          = module.vpc.private_subnets
#     security_groups  = [aws_security_group.ecs_service.id]
#     assign_public_ip = false
#   }

#   tags = var.tags
# }

# # Output ECS cluster information
# output "ecs_cluster_name" {
#   value = aws_ecs_cluster.main.name
# }

# output "ecs_service_name" {
#   value = aws_ecs_service.tailscale_demo_streamer.name
# }
