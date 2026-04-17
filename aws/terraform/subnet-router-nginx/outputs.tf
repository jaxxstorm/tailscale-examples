output "ec2_instance_id" {
  value = aws_instance.subnet-router.id
}

output "private_nginx_instance_id" {
  value = aws_instance.private-nginx.id
}

output "private_nginx_private_ip" {
  value = aws_instance.private-nginx.private_ip
}
