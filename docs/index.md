# Welcome

The **Terraform DatafyAWS Provider** is a fork of the official [Terraform AWS Provider](https://github.com/hashicorp/terraform-provider-aws) maintained by [Datafy](https://datafy.io). It includes all the functionality of the standard AWS provider with added support for using [Datafy](https://datafy.io) when managing AWS resources with Terraform.

Because this provider is fully compatible with the upstream AWS provider, the [official Terraform AWS Provider documentation](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) applies here as well — refer to it for resource and data source reference material.

## Configuration

The `provider "datafyaws"` block requires **all the same configuration** as the [`provider "aws"` block](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#provider-configuration). The only addition is `datafy_token`, which can also be set via the `DATAFY_TOKEN` environment variable.

_To migrate, duplicate your existing `provider "aws"` block as `provider "datafyaws"` and add `datafy_token`._

## Resources Supported by Datafy

Add `provider = datafyaws` to resources of the following types to enable Datafy support:

- `aws_ebs_volume`
- `aws_volume_attachment`

## Example Usage

```hcl
terraform {
  required_providers {
    datafyaws = {
      source  = "datafy-io/datafyaws"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region     = "us-east-1"
  access_key = "YOUR_ACCESS_KEY"
  secret_key = "YOUR_SECRET_KEY"
}

# All the same configuration as provider "aws", plus datafy_token
provider "datafyaws" {
  region     = "us-east-1"
  access_key = "YOUR_ACCESS_KEY"
  secret_key = "YOUR_SECRET_KEY"

  datafy_token = "YOUR_DATAFY_TOKEN" # or set DATAFY_TOKEN env var
}

resource "aws_ebs_volume" "example" {
  provider = datafyaws

  availability_zone = "us-east-1a"
  size              = 40

  tags = {
    Name = "example-volume"
  }
}

resource "aws_volume_attachment" "example" {
  provider = datafyaws

  device_name = "/dev/sdh"
  volume_id   = aws_ebs_volume.example.id
  instance_id = "i-1234567890abcdef0"
}
```
