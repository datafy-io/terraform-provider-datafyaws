<!-- Copyright IBM Corp. 2014, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

# DatafyAWS Provider

The **DatafyAWS provider** is a build of the official [Terraform AWS Provider](https://github.com/hashicorp/terraform-provider-aws) maintained by [Datafy](https://datafy.io). It contains every resource and data source of `hashicorp/aws` unchanged, plus Datafy-aware behavior for `aws_ebs_volume` and `aws_volume_attachment`.

Use it side by side with `hashicorp/aws`: keep `hashicorp/aws` as your default provider, and set `provider = datafyaws` only on the EBS volume and volume attachment resources that Datafy may manage. For any argument or attribute not described in these pages, the [hashicorp/aws documentation](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) applies.

## Why a separate provider

When Datafy optimizes ("datafies") an EBS volume, it replaces the original volume with Datafy-managed volumes and deletes the original. Terraform state still holds the original volume ID. With `hashicorp/aws`, the next plan sees the volume as gone and tries to create it again. The DatafyAWS provider asks the Datafy API about each volume and, when Datafy manages it, keeps the state stable and routes supported changes through Datafy.

## Versions

This version of DatafyAWS is built on `hashicorp/aws` 6.42, for use alongside `hashicorp/aws` 6.x. A version number carries both the `hashicorp/aws` version and the Datafy version, in the form `<MAJOR>.<MINOR>.<PATCH>`:

- `MAJOR` is the `hashicorp/aws` major version.
- `MINOR` is the `hashicorp/aws` minor × 100 + the Datafy minor. `6.4204.0` is `hashicorp/aws` 6.42.0 with Datafy 1.4.0.
- `PATCH` is the `hashicorp/aws` patch × 100 + the Datafy patch. `6.4204.101` is `hashicorp/aws` 6.42.1 with Datafy 1.4.1.

To decode `MINOR` or `PATCH`, split it from the right: the last two digits are the Datafy part, and the rest is the `hashicorp/aws` part (empty means 0). Every release note states the decoded versions, for example "AWS 6.42.1 / Datafy 1.4.1". Versions always increase, whichever part changes.

Version constraints work as they do for `hashicorp/aws`:

- `"~> 6.4204"` picks up every 6.x release from `hashicorp/aws` 6.42 with Datafy 1.4 onward, like `~> 6.42` does for `hashicorp/aws`.
- `"~> 6.4204.0"` stays on `hashicorp/aws` 6.42 with Datafy 1.4 and picks up only patch releases.
- `">= 6.4200.0, < 6.4300.0"` stays on `hashicorp/aws` 6.42 with any Datafy version.

Releases up to and including `6042.4.0` used an earlier scheme (`6042` for `hashicorp/aws` 6.42) and do not match these constraints. Replace a constraint such as `"~> 6042.0"` with `"~> 6.4204"`.

The resources and provider-block arguments available through `provider "datafyaws"` are those of `hashicorp/aws` 6.42, even if your `hashicorp/aws` provider is newer. An argument that `hashicorp/aws` added later is rejected in a `provider "datafyaws"` block.

## Configuration

The `provider "datafyaws"` block accepts every argument of the [`provider "aws"` block](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#provider-configuration), plus:

- `datafy_token` (Optional, Sensitive) - Datafy API token. Can also be set with the `DATAFY_TOKEN` environment variable. Requests to the Datafy API without a token fail.
- `datafy_url` (Optional) - Datafy API endpoint. Defaults to `https://iac.datafy.io`. Can also be set with the `DATAFY_URL` environment variable. Change it only when Datafy support asks you to.

Copy every argument of your `provider "aws"` block into the `provider "datafyaws"` block exactly as it is (`region`, `profile`, `assume_role`, `default_tags`, `ignore_tags`, `alias`, and any others), then add `datafy_token`. If you have several `provider "aws"` blocks (for example, one per region or account), create a matching `provider "datafyaws"` block for each one that manages EBS volumes.

## Resources with Datafy support

Set `provider = datafyaws` on:

- [`aws_ebs_volume`](https://registry.terraform.io/providers/datafy-io/datafyaws/latest/docs/resources/ebs_volume)
- [`aws_volume_attachment`](https://registry.terraform.io/providers/datafy-io/datafyaws/latest/docs/resources/volume_attachment)

Every other resource and data source in DatafyAWS behaves exactly like the same one in `hashicorp/aws`. Use `hashicorp/aws` for them.

### Datafy-managed volumes

A volume is **Datafy-managed** while Datafy has datafied it: the volume ID in your state is kept as a logical ID, and its data lives on Datafy-managed EBS volumes in your account. For such a volume:

- Refresh does not report drift. `size`, `iops`, `throughput`, and `tags` are read from Datafy and the Datafy-managed volumes; every other attribute stays as it is in state.
- When Datafy hands the volume back (un-datafy), it creates a new, regular EBS volume. On the next refresh the provider moves the resource to the new volume ID with a warning, and from then on manages it like any EBS volume.
- Only `size`, `iops`, `throughput`, and `tags` can change. Datafy applies size, IOPS, and throughput changes, and Terraform waits until Datafy reports the new value. Tag changes are applied to the Datafy-managed volumes.
- Any other change, including changes that would replace the volume, fails at plan time with `can't modify datafied EBS Volume`.
- Any change to an `aws_volume_attachment` of a managed volume fails at plan time.

## Example usage

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    datafyaws = {
      source  = "datafy-io/datafyaws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region  = "us-east-1"
  profile = "my-profile"
}

# Same configuration as provider "aws". The Datafy token is read from the
# DATAFY_TOKEN environment variable.
provider "datafyaws" {
  region  = "us-east-1"
  profile = "my-profile"
}

resource "aws_instance" "example" {
  ami           = "ami-0123456789abcdef0"
  instance_type = "m7i.large"
}

resource "aws_ebs_volume" "example" {
  provider = datafyaws

  availability_zone = aws_instance.example.availability_zone
  type              = "gp3"
  size              = 40

  tags = {
    Name = "example-volume"
  }
}

resource "aws_volume_attachment" "example" {
  provider = datafyaws

  device_name = "/dev/sdh"
  volume_id   = aws_ebs_volume.example.id
  instance_id = aws_instance.example.id
}
```
