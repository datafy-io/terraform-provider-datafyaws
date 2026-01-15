DATAFY_PROJECT_NAME := terraform-provider-datafyaws
DATAFY_MODIFIED_PACKAGES= ./internal/datafy ./internal/service/ec2 ./internal/provider

default: datafy-build

.PHONY: datafy-build
datafy-build: build datafy-rename-bin

.PHONY: datafy-install
datafy-install: install datafy-rename-bin

.PHONY: datafy-rename-bin
datafy-rename-bin:
	@mv ~/go/bin/terraform-provider-aws ~/go/bin/$(DATAFY_PROJECT_NAME)

datafy-test:
	@echo "make: Running unit tests..."
	$(GO_VER) test -count $(TEST_COUNT) $(DATAFY_MODIFIED_PACKAGES) $(TESTARGS) -timeout=5m

.PHONY: datafy-rebase
datafy-rebase:
	@echo "make: Update CODEOWNERS..."
	@echo "* @datafy-io/saas" > CODEOWNERS

	@echo "make: Removing files..."
	@rm -rf mkdocs.yml ROADMAP.md
	@rm -rf website
	@rm -rf internal/generate/allowsubcats internal/generate/checknames internal/generate/customends

	@EXCLUDE=("datafy-pull-request.yml" "datafy-release.yml") ; \
	PATTERN=$$(printf "! -name %s " "$${EXCLUDE[@]}") ; \
	find ./.github/workflows $$PATTERN -mindepth 1 -maxdepth 1 -exec rm -rf {} + > /dev/null 2>&1;

	@EXCLUDE=("index.md") ; \
	PATTERN=$$(printf "! -name %s " "$${EXCLUDE[@]}") ; \
	find ./docs $$PATTERN -mindepth 1 -maxdepth 1 -exec rm -rf {} + > /dev/null 2>&1;
