Here's the fixed Markdown version:

# Pulumi Example: Deploying an AWS Environment with Tailscale Integration

This project demonstrates the use of Pulumi to provision AWS infrastructure with Tailscale integration. It includes the creation of a VPC, IAM roles, EC2 instances, and an auto-scaling group with Tailscale configuration using cloud-init.

## Features

- **VPC Setup**: A configurable VPC with public and private subnets.
- **IAM Roles and Policies**: Roles for EC2 instances with policies for SSM, CloudWatch, and EC2.
- **Tailscale Integration**: Automated setup of Tailscale using templates and cloud-init.
- **Autoscaling**: An auto-scaling group ensures high availability.
- **Security Groups**: Proper security configurations for subnet and instance communication.
- **Cloud-init**: Templates for Tailscale installation and configuration.

## Prerequisites

1. Pulumi CLI installed.
2. AWS CLI installed and configured with appropriate credentials.
3. Node.js (v16+ recommended).
4. A valid Tailscale OAuth Client Secret, securely configured in the project.
5. A Tailscale oauth client secret. The client secret must have the Devices Core - Write and Auth Key - Write scope and match the tags you're using

## Setup

1. **Install Dependencies**: Run `npm install`.

2. **Prepare Templates**:
   - `templates/install_tailscale.sh.tmpl`: Script to install Tailscale.
   - `templates/setup_tailscale.sh.tmpl`: Script to configure Tailscale.
   - `files/ip_forwarding.sh`: Script to enable IP forwarding.

3. **Deploy the Infrastructure**:
   Run the following command to deploy the stack:
   ```
   pulumi config set tailscaleOauthClientSecret "value" --secret
   pulumi up
   ```

4. **Destroy the Infrastructure**:
   To tear down the stack, run:
   ```
   pulumi destroy
   ```

## Project Structure

- **index.ts**: Main Pulumi program defining the infrastructure.
- **templates/**: Directory containing Tailscale installation and setup scripts.
- **files/**: Additional scripts used in the deployment.
- **Pulumi.<stack-name>.yaml**: Configuration file for the Pulumi stack.

## Key Components

### VPC
A VPC with public and private subnets for scalable, secure deployments.

### IAM Roles
IAM roles configured with necessary policies for EC2 and CloudWatch.

### Launch Template
Automatically configures instances using the latest Ubuntu AMI and sets up Tailscale using cloud-init.

### Auto-Scaling Group
Ensures high availability with rolling updates for seamless instance refresh.

### Security Groups
Allows internal communication and enables necessary Tailscale ports.

## Configuration Templates

The project uses templates for generating cloud-init configurations dynamically:
- **install_tailscale.sh.tmpl**: Script for installing Tailscale.
- **setup_tailscale.sh.tmpl**: Script for configuring Tailscale.

## Example Usage

This setup provides an EC2 environment automatically integrated with Tailscale. Instances in the auto-scaling group will join your Tailscale network and advertise tags for simplified routing and access control.

## Notes

- Customize instance types, AMI ID, and configurations to suit your requirements.
- Ensure the Tailscale OAuth client secret is securely managed in your Pulumi configuration.
- The `ip_forwarding.sh` script is optional but recommended for routing use cases.
