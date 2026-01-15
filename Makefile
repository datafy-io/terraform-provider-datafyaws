DATAFY_PROJECT_NAME := terraform-provider-datafyaws

define echoblue
	@printf "\033[0;34m%s\033[0m\n" $1
endef

.DEFAULT_GOAL := init
.PHONY: init git

init: git

git:
	$(call echoblue, "==> Configuring git for $(DATAFY_PROJECT_NAME)")
	@git remote get-url hashicorp >/dev/null 2>&1 || git remote add hashicorp "git@github.com:hashicorp/terraform-provider-aws.git" \
		|| echo "ERROR: failed to add the hashicorp remote"
	@git fetch origin || echo "ERROR: failed to fetch origin"

	$(call echoblue, "==> Setting up v4 branches...")
	@tag=$$(git show "origin/release/4.x:.aws-version") \
		&& echo "release/4.x is based on $$tag" \
		&& git fetch --no-tags hashicorp "refs/tags/$$tag:refs/tags/$$tag" \
		&& git branch -f "base/release/4.x" "refs/tags/$$tag" \
		|| echo "ERROR: failed to set up base/release/4.x"

	$(call echoblue, "==> Setting up v5 branches...")
	@tag=$$(git show "origin/release/5.x:.aws-version") \
		&& echo "release/5.x is based on $$tag" \
		&& git fetch --no-tags hashicorp "refs/tags/$$tag:refs/tags/$$tag" \
		&& git branch -f "base/release/5.x" "refs/tags/$$tag" \
		|| echo "ERROR: failed to set up base/release/5.x"

	$(call echoblue, "==> Setting up v6 branches...")
	@tag=$$(git show "origin/release/6.x:.aws-version") \
		&& echo "release/6.x is based on $$tag" \
		&& git fetch --no-tags hashicorp "refs/tags/$$tag:refs/tags/$$tag" \
		&& git branch -f "base/release/6.x" "refs/tags/$$tag" \
		|| echo "ERROR: failed to set up base/release/6.x"
