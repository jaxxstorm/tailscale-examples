output "aws_instance_east" {
    value = aws_instance.east.id
}

output "nlb_ip" {
    value = aws_eip.nlb_eip[0].public_ip
}