output "aws_instance_east" {
  value = aws_instance.east.id
}

# output "aws_instance_west" {
#   value = aws_instance.west.id
# }

output "nlb_eip_public_ips" {
  description = "List of public IP addresses of the NLB Elastic IPs"
  value       = aws_eip.nlb_eip[*].public_ip
}