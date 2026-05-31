<!-- Copyright IBM Corp. 2014, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

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
      version = "~> 6.0"
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

| Contribution Guide | Description |
|--------------------|-------------|
| [Small Changes](bugs-and-enhancements.md) | Requirements for small additions or bug-fixes on existing resources/data sources |
| [Resources](add-a-new-resource.md) | Allow the management of a logical resource within AWS by adding a new resource to the Terraform AWS Provider. |
| [Data Source](add-a-new-datasource.md) | Let your Terraform configurations use data from resources not under local management by creating ready only data sources. |
| [Services](add-a-new-service.md) | Allow Terraform (via the AWS Provider) to manage an entirely new AWS service by introducing the resources and data sources required to manage configuration of the service. |
| [AWS Region](add-a-new-region.md) | New regions are immediately usable with the provider with the caveat that a configuration workaround is required to skip validation of the region during cli operations. A small set of changes are required to make this workaround necessary. |
| [Resource Name Generation](resource-name-generation.md) | Allow a resource to either fully, or partially, generate its own resource names. This can be useful in cases where the resource name uniquely identifies the resource and it needs to be recreated. It can also be used when a name is required, but the specific name is not important. |
| [Tagging Support](resource-tagging.md) | Many AWS resources allow assigning metadata via tags. However, frequently AWS services are launched without tagging support so this will often need to be added later. |
| [Import Support](add-import-support.md) | Adding import support allows `terraform import` to be run targeting an existing unmanaged resource and pulling its configuration into Terraform state. Typically import support is added during initial resource implementation but in some cases this will need to be added later. |
| [Enhanced Region Support](enhanced-region-support.md) | Most AWS resources are Regional – they are created and exist in a single AWS Region. By default Regional resources have a top-level `region` argument that allows the Region to be configured. |
| [End User Documentation](end-user-documentation.md)| The provider documentation is displayed on the [Terraform Registry](https://registry.terraform.io/providers/hashicorp/aws/latest) and is sourced and refreshed from the provider repository during the release process. |

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
