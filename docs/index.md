---
layout: "aws"
page_title: "Provider: DATAFYAWS"
description: |-
  The Datafy AWS provider enables Terraform-managed EBS volumes, volume attachments, and EC2 instances to be integrated with the Datafy platform. Datafy optimizes cloud storage costs by automatically managing volume lifecycles. You must configure the provider with both AWS credentials and a Datafy token before you can use it.
---

# DATAFYAWS Provider

The Datafy AWS provider (`datafyaws`) is a drop-in replacement for the standard [AWS provider](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) that adds [Datafy](https://docs.datafy.io) volume management capabilities to your Terraform-managed AWS infrastructure.

When you use the `datafyaws` provider instead of the standard `aws` provider on supported resources, Datafy can automatically manage your EBS volume lifecycle — including snapshot-based restores, volume replacements, and cost optimization — without requiring changes to your existing Terraform resource definitions.

## Prerequisites

Before using this provider, you need:

1. **A Datafy organization account** — contact the Datafy team to set up your organization.
2. **A Datafy API token** — generate a token from the [Datafy UI](https://docs.datafy.io). This token authenticates the provider with the Datafy API.
3. **AWS credentials** — the same credentials you use with the standard AWS provider (access keys, IAM role, instance profile, etc.).

## Example Usage

### Terraform 0.13+

```terraform
terraform {
  required_providers {
    datafyaws = {
      source  = "datafy-io/datafyaws"
      version = "~> 1.0"
    }
  }
}

# Configure the Datafy AWS Provider
provider "datafyaws" {
  region      = "us-east-1"
  datafy_token = var.datafy_token  # Or set via DATAFY_TOKEN environment variable
}

# Create an EBS Volume managed by Datafy
resource "aws_ebs_volume" "example" {
  provider = datafyaws

  availability_zone = "us-east-1a"
  size              = 40

  tags = {
    Name = "my-datafy-volume"
  }
}

# Attach the volume to an instance
resource "aws_volume_attachment" "example" {
  provider = datafyaws

  device_name = "/dev/sdf"
  volume_id   = aws_ebs_volume.example.id
  instance_id = aws_instance.example.id
}

# EC2 Instance with Datafy integration
resource "aws_instance" "example" {
  provider = datafyaws

  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t3.medium"
}
```

### Terraform 0.12 and earlier

```terraform
provider "datafyaws" {
  version      = "~> 1.0"
  region       = "us-east-1"
  datafy_token = var.datafy_token
}

resource "aws_ebs_volume" "example" {
  provider = datafyaws

  availability_zone = "us-east-1a"
  size              = 40
}
```

## Provider Configuration

The `datafyaws` provider accepts all the same configuration options as the [AWS provider](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#authentication-and-configuration), plus the following Datafy-specific attributes:

### Datafy-Specific Arguments

* `datafy_token` - (Optional) The Datafy API token used to authenticate with the Datafy platform. Can also be set via the `DATAFY_TOKEN` environment variable. This token is required for Datafy volume management features to work.
* `datafy_url` - (Optional) The Datafy API endpoint URL. Can also be set via the `DATAFY_URL` environment variable. Defaults to `https://iac.datafy.io`. You should not need to change this unless directed by the Datafy team.

### AWS Authentication Arguments

All standard AWS provider authentication methods are supported. See the [AWS provider documentation](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#authentication-and-configuration) for the full list. Common options include:

* `region` - (Optional) The AWS region for API operations (e.g., `us-east-1`, `eu-west-1`).
* `access_key` - (Optional) AWS access key ID. Can also be set via `AWS_ACCESS_KEY_ID`.
* `secret_key` - (Optional) AWS secret access key. Can also be set via `AWS_SECRET_ACCESS_KEY`.
* `profile` - (Optional) AWS shared credentials profile name.
* `assume_role` - (Optional) Configuration block for assuming an IAM role.
* `default_tags` - (Optional) Configuration block for default tags applied to all resources.

## Resources Supported by Datafy

The Datafy provider should be added to **every resource** of the following types to enable Datafy management:

* **`aws_ebs_volume`** — EBS volumes. When managed by Datafy, the platform controls the volume lifecycle including snapshot-based restores and cost-optimized replacements. Once a volume is managed by Datafy, only tag updates can be made through Terraform — all other modifications (size, IOPS, throughput) are handled by the Datafy platform.
* **`aws_volume_attachment`** — Volume attachments. When the underlying volume is managed by Datafy, the provider coordinates attach/detach operations through the Datafy API to ensure consistency.
* **`aws_instance`** — EC2 instances. The provider integrates with Datafy to track block device mappings for instances with Datafy-managed volumes.

### Important Behavior Notes

- **Managed volumes**: Once Datafy takes control of a volume (i.e., the volume is "datafied"), Terraform can only update tags on that volume. All other properties are managed by the Datafy platform.
- **Volume replacement**: When a volume is "undatafied" (removed from Datafy management), a new replacement volume may be created. The provider automatically updates the Terraform state to point to the new volume ID.
- **Snapshot restores**: Datafy can create volumes from its own snapshots. The provider handles the coordination between Datafy snapshot IDs and AWS volume IDs.
- **Datafy tags**: The provider automatically filters out internal Datafy tags (prefixed with `datafy:` and the `Managed-By: Datafy.io` tag) so they do not appear in your Terraform state.

Other AWS resource types are not managed by Datafy. Using the `datafyaws` provider on unsupported resource types will behave identically to the standard AWS provider.

## Further Reading

- [Datafy Documentation](https://docs.datafy.io) — full platform documentation, including setup guides and best practices.
