# DatafyAWS Provider

The **DatafyAWS provider** is a build of the official [Terraform AWS Provider](https://github.com/hashicorp/terraform-provider-aws) maintained by [Datafy](https://datafy.io). It contains every resource and data source of `hashicorp/aws` unchanged, plus Datafy-aware behavior for `aws_ebs_volume` and `aws_volume_attachment`.

Use it side by side with `hashicorp/aws`: keep `hashicorp/aws` as your default provider, and set `provider = datafyaws` only on the EBS volume and volume attachment resources that Datafy may manage. For any argument or attribute not described on this page, the [hashicorp/aws documentation](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) applies.

## Why a separate provider

When Datafy optimizes ("datafies") an EBS volume, it replaces the original volume with Datafy-managed volumes and deletes the original. Terraform state still holds the original volume ID. With `hashicorp/aws`, the next plan sees the volume as gone and tries to create it again; destroying it does not clean up the Datafy-managed volumes. The DatafyAWS provider asks the Datafy API about each volume and, when Datafy manages it, keeps the state stable and routes supported changes through Datafy.

## Versions

This version of DatafyAWS is built on `hashicorp/aws` 5.100.0, for use alongside `hashicorp/aws` 5.x. A version number has the form `<AWS_MAJOR><AWS_MINOR:3 digits>.<DATAFY_MINOR>.<DATAFY_PATCH>`:

- The first number encodes the `hashicorp/aws` release the build is based on: major × 1000 + minor. `5100` is built on `hashicorp/aws` 5.100.0.
- The last two numbers are the Datafy release: `v5100.4.0` is Datafy release 4.0 on `hashicorp/aws` 5.100.0.

Use the version constraint `"~> 5100"`. It picks up new Datafy releases (`5100.4.0`, `5100.5.0`, …). Constraints such as `~> 5.0` or `~> 5` do **not** match any DatafyAWS release.

The resources and provider-block arguments available through `provider "datafyaws"` are those of `hashicorp/aws` 5.100.0, even if your `hashicorp/aws` provider is newer. An argument that `hashicorp/aws` added after 5.100.0 is rejected in a `provider "datafyaws"` block.

## Configuration

The `provider "datafyaws"` block accepts every argument of the [`provider "aws"` block](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#provider-configuration) of its base version, plus:

- `datafy_token` (Optional, Sensitive) - Datafy API token. Can also be set with the `DATAFY_TOKEN` environment variable. Required in practice; see [Running without a Datafy token](#running-without-a-datafy-token).
- `datafy_url` (Optional) - Datafy API endpoint. Defaults to `https://iac.datafy.io`. Can also be set with the `DATAFY_URL` environment variable. Change it only when Datafy support asks you to.

To migrate, copy your existing `provider "aws"` block (including any `alias`, `region`, `assume_role`, `default_tags`, and `ignore_tags`) into a `provider "datafyaws"` block and add the token.

### Running without a Datafy token

Without a token the provider starts and prints the warning `Datafy Token was not found for provider`, but every call to the Datafy API is rejected. The provider checks every `aws_ebs_volume` and `aws_volume_attachment` with the Datafy API, whether or not Datafy manages it, so without a token:

| Operation | Result |
|---|---|
| `terraform plan` | Succeeds, but cannot detect Datafy-managed volumes, so changes that Datafy would reject at plan time are only rejected at apply time. |
| Create `aws_ebs_volume` (plain or from an EBS `snap-` snapshot) | Works. |
| Create `aws_ebs_volume` with `autoscaling_native = true` or from a `dsnap-` snapshot | Fails. |
| Update `aws_ebs_volume` (any argument, tags included) | Fails. |
| Destroy `aws_ebs_volume` | Fails. |
| Create or destroy `aws_volume_attachment` | Fails. |
| Refresh a volume or attachment that no longer exists in AWS | Fails instead of removing it from state. |

## Resources with Datafy support

Set `provider = datafyaws` on:

- `aws_ebs_volume`
- `aws_volume_attachment`

Every other resource and data source in DatafyAWS behaves exactly like the same one in `hashicorp/aws`. Use `hashicorp/aws` for them.

### Datafy-managed volumes

A volume is **Datafy-managed** while Datafy has datafied it: the volume ID in your state is kept as a logical ID, and its data lives on Datafy-managed EBS volumes in your account. For such a volume:

- Refresh does not report drift. `size`, `iops`, `throughput`, and `tags` are read from Datafy and the Datafy-managed volumes; every other attribute stays as it is in state.
- When Datafy hands the volume back (un-datafy), it creates a new, regular EBS volume. On the next refresh the provider moves the resource to the new volume ID with a warning, and from then on manages it like any EBS volume.
- Only `size`, `iops`, `throughput`, and `tags` can change. Datafy applies size, IOPS, and throughput changes, and Terraform waits until Datafy reports the new value. Tag changes are applied to the Datafy-managed volumes.
- Any other change, including changes that would replace the volume, fails at plan time with `can't modify datafied EBS Volume`.
- Any change to an `aws_volume_attachment` of a managed volume fails at plan time.

## aws_ebs_volume

### Argument: autoscaling_native

`autoscaling_native` (Optional, Boolean) creates the volume directly as a Datafy autoscaling volume instead of as a regular EBS volume that Datafy datafies later.

```hcl
resource "aws_ebs_volume" "data" {
  provider = datafyaws

  availability_zone  = "us-east-1a"
  type               = "gp3"
  size               = 500
  iops               = 6000 # optional
  throughput         = 250  # optional
  autoscaling_native = true

  tags = {
    Name = "data"
  }
}
```

- `type` must be `"gp3"`.
- Datafy receives `availability_zone`, `size`, `iops`, `throughput`, `encrypted`, `kms_key_id`, and `tags`. Do not combine `autoscaling_native` with `snapshot_id`, `multi_attach_enabled`, or `outpost_arn`; they are not applied to a native volume.
- The volume is Datafy-managed from creation; see [Datafy-managed volumes](#datafy-managed-volumes).
- The argument is immutable. It cannot be added to an existing volume, set to `false`, or changed in any way. The only allowed change is removing it from the configuration, and only after the volume is no longer datafied.

### Restoring from a Datafy snapshot

To create a volume from a Datafy snapshot, set `snapshot_id` to the Datafy snapshot ID (`dsnap-…`):

```hcl
resource "aws_ebs_volume" "restored" {
  provider = datafyaws

  availability_zone = "us-east-1a"
  snapshot_id       = "dsnap-0123456789abcdef0"
}
```

- Datafy receives `availability_zone`, `iops`, `throughput`, and `tags`. The size, type, and encryption come from the snapshot. If you set `size` or `type`, set them to the values the restored volume has, otherwise the next plan shows a change that Datafy rejects.
- The restored volume is Datafy-managed; see [Datafy-managed volumes](#datafy-managed-volumes).
- A Datafy snapshot is backed by EBS snapshots (`snap-…`) in your account. Using one of those EBS snapshots as `snapshot_id` fails at apply time with an error naming the Datafy snapshot to use instead.

### Tags

- Tag keys that start with `datafy:` are reserved for Datafy. Setting one in `tags` fails at plan time.
- Datafy's own tags on the Datafy-managed volumes (keys starting with `datafy:`, and `Managed-By = Datafy.io`) are not stored in `tags` or `tags_all`, so they never show up as drift.

### final_snapshot

`final_snapshot` is not supported for Datafy-managed volumes. On destroy the provider tries to snapshot the original volume ID, which either fails (the original volume no longer exists) or snapshots the original volume, not the data Datafy manages. Because a managed volume rejects every argument change other than `size`, `iops`, `throughput`, and `tags`, `final_snapshot` cannot be switched off after Datafy has datafied the volume. Leave `final_snapshot` unset on volumes Datafy may manage, and use Datafy snapshots instead.

### Destroy

Destroying a Datafy-managed volume deletes the Datafy-managed volumes (and the original volume, if it still exists).

## aws_volume_attachment

For a Datafy-managed volume:

- Create asks Datafy to attach the volume. Datafy attaches every Datafy-managed volume behind it to the instance, and Terraform waits until all of them are attached.
- Refresh does not report drift while Datafy manages the volume. When the volume is un-datafied, `volume_id` moves to the new volume ID with a warning.
- Any change fails at plan time.
- Destroy asks Datafy to detach the volume, and Terraform waits until every Datafy-managed volume is detached. `force_detach` and `stop_instance_before_detaching` are not applied.

## aws_instance and other resources

`aws_instance` has no Datafy-specific behavior in this provider; using `provider = datafyaws` on it gives the same result as `hashicorp/aws`. Two `aws_instance` features interact with volumes Datafy may manage, with either provider:

- **`ebs_block_device`.** When Datafy datafies a volume declared in an instance's `ebs_block_device` block, the original volume disappears from the instance and the Datafy-managed volumes appear in its place. Because `ebs_block_device` changes force replacement, Terraform plans to **replace the instance**. Declare volumes that Datafy may manage as separate `aws_ebs_volume` and `aws_volume_attachment` resources with `provider = datafyaws`, not in `ebs_block_device`.
- **`volume_tags`.** `volume_tags` is read from every volume attached to the instance, including Datafy-managed volumes and their `datafy:` and `Managed-By` tags. Terraform then plans to remove those tags, and applying the plan removes them from the Datafy-managed volumes. If you use `volume_tags`, add the Datafy tags to `ignore_tags` in the provider that manages the instance:

  ```hcl
  provider "aws" {
    ignore_tags {
      keys         = ["Managed-By"]
      key_prefixes = ["datafy:"]
    }
  }
  ```

## Example usage

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    datafyaws = {
      source  = "datafy-io/datafyaws"
      version = "~> 5100"
    }
  }
}

# Credentials come from the standard AWS sources (environment variables,
# shared config/credentials files, instance or task roles).
provider "aws" {
  region = "us-east-1"
}

# Same configuration as provider "aws". The Datafy token is read from the
# DATAFY_TOKEN environment variable.
provider "datafyaws" {
  region = "us-east-1"
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
