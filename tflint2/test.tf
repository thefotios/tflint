variable "type" {
    default = "t1.2xlarge"
}

resource "aws_instance" "web" {
  ami           = "ami-b73b63a0"
  instance_type = "${var.type}"
  tags {
    Name = "HelloWorld"
  }
}

module "test" {
  source = "./module1"
  foo = "${var.type}"
}
