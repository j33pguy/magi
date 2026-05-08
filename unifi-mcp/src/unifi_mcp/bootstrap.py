"""Bootstrap the Terraform working directory with provider config.

Writes:
  - versions.tf  (terraform + required_providers block)
  - provider.tf  (configured provider, reads creds from env vars)

The provider credentials are read by the provider itself from UNIFI_USERNAME /
UNIFI_PASSWORD / UNIFI_API / UNIFI_SITE / UNIFI_INSECURE — same env vars the
MCP server uses for its own client, so a single .env covers both.
"""

from __future__ import annotations

from pathlib import Path

# Default to the actively-maintained community fork. paultyng/terraform-provider-unifi
# was archived in 2025; resource syntax is identical, so this is a drop-in.
DEFAULT_PROVIDER_SOURCE = "ubiquiti-community/unifi"
DEFAULT_PROVIDER_VERSION = "~> 0.41"


VERSIONS_TF = """\
terraform {{
  required_version = ">= 1.5.0"

  required_providers {{
    unifi = {{
      source  = "{source}"
      version = "{version}"
    }}
  }}
}}
"""

PROVIDER_TF = """\
# Provider reads credentials from environment:
#   UNIFI_USERNAME, UNIFI_PASSWORD, UNIFI_API, UNIFI_SITE, UNIFI_INSECURE
provider "unifi" {}
"""


def ensure_bootstrap(
    tf_dir: Path,
    *,
    provider_source: str = DEFAULT_PROVIDER_SOURCE,
    provider_version: str = DEFAULT_PROVIDER_VERSION,
) -> list[Path]:
    """Create versions.tf / provider.tf if missing. Returns the files written."""
    tf_dir.mkdir(parents=True, exist_ok=True)
    written: list[Path] = []

    versions = tf_dir / "versions.tf"
    if not versions.exists():
        versions.write_text(
            VERSIONS_TF.format(source=provider_source, version=provider_version)
        )
        written.append(versions)

    provider = tf_dir / "provider.tf"
    if not provider.exists():
        provider.write_text(PROVIDER_TF)
        written.append(provider)

    gitignore = tf_dir / ".gitignore"
    if not gitignore.exists():
        gitignore.write_text(
            "# Terraform working files — do not commit.\n"
            ".terraform/\n"
            ".terraform.lock.hcl\n"
            "*.tfstate\n"
            "*.tfstate.*\n"
            "tfplan\n"
            "crash.log\n"
        )
        written.append(gitignore)

    return written
