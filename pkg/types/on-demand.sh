#!/bin/bash

set -euo pipefail

# shortcut mirroring if the source registry is the same as the target ACR
REQUIRED_REGISTRY_VARS=("TARGET_ACR" "SOURCE_REGISTRY")
for VAR in "${REQUIRED_REGISTRY_VARS[@]}"; do
    if [[ -z "${!VAR}" ]]; then
        echo "Error: Environment variable $VAR is not set."
        exit 1
    fi
done
ACR_DOMAIN_SUFFIX="$(az cloud show --query "suffixes.acrLoginServerEndpoint" --output tsv)"
if [[ "${SOURCE_REGISTRY}" == "${TARGET_ACR}${ACR_DOMAIN_SUFFIX}" ]]; then
    echo "Source and target registry are the same. No mirroring needed."
    exit 0
fi

# validate
REQUIRED_VARS=("REPOSITORY" "DIGEST")
for VAR in "${REQUIRED_VARS[@]}"; do
    if [[ -z "${!VAR}" ]]; then
        echo "Error: Environment variable $VAR is not set."
        exit 1
    fi
done

# validate authentication variables based on AUTH_USING flag
case "${AUTH_USING:-credential}" in
    "msi")
        echo "Using MSI authentication mode."
        ;;
    "credential")
        echo "Using credential authentication mode."
        # When using credentials, PULL_SECRET_KV and PULL_SECRET are required
        AUTH_REQUIRED_VARS=("PULL_SECRET_KV" "PULL_SECRET")
        for VAR in "${AUTH_REQUIRED_VARS[@]}"; do
            if [[ -z "${!VAR}" ]]; then
                echo "Error: Environment variable $VAR is required when AUTH_USING='credential'."
                exit 1
            fi
        done
        ;;
    *)
        echo "Error: AUTH_USING must be either 'msi' or 'credential'. Got: '${AUTH_USING}'"
        exit 1
        ;;
esac

# create temporary FS structure
TMP_DIR="$(mktemp -d)"
CONTAINERS_DIR="${TMP_DIR}/containers"
AUTH_JSON="${CONTAINERS_DIR}/auth.json"
ORAS_CACHE="${TMP_DIR}/oras-cache"
mkdir -p "${CONTAINERS_DIR}"
mkdir -p "${ORAS_CACHE}"
trap 'rm -rf ${TMP_DIR}' EXIT

# get authentication for source registry
case "${AUTH_USING:-credential}" in
    "msi")
        # use MSI access token for source registry
        echo "Using MSI access token for source registry ${SOURCE_REGISTRY}."
        
        # Get MSI access token for the source ACR
        if output="$( az acr login --name "${SOURCE_REGISTRY}" --expose-token --only-show-errors 2>&1 )"; then
            SOURCE_RESPONSE="${output}"
        else
            echo "Failed to log in to source ACR ${SOURCE_REGISTRY}: ${output}"
            exit 1
        fi
        SOURCE_ACR_LOGIN_SERVER="$(jq --raw-output .loginServer <<<"${SOURCE_RESPONSE}" )"
        SOURCE_ACCESS_TOKEN="$(jq --raw-output .accessToken <<<"${SOURCE_RESPONSE}" )"
        
        # Create auth.json with MSI token for source registry
        cat > "${AUTH_JSON}" << EOF
{
    "auths": {
        "${SOURCE_ACR_LOGIN_SERVER}": {
            "auth": "$(echo -n "00000000-0000-0000-0000-000000000000:${SOURCE_ACCESS_TOKEN}" | base64 -w 0)"
        }
    }
}
EOF
        ;;
    "credential")
        # use pull secret from Key Vault
        echo "Fetch pull secret for source registry ${SOURCE_REGISTRY} from ${PULL_SECRET_KV} KV."
        az keyvault secret download --vault-name "${PULL_SECRET_KV}" --name "${PULL_SECRET}" -e base64 --file "${AUTH_JSON}"
        ;;
esac

# ACR login to target registry
echo "Logging into target ACR ${TARGET_ACR}."
if output="$( az acr login --name "${TARGET_ACR}" --expose-token --only-show-errors --output json 2>&1 )"; then
  RESPONSE="${output}"
else
  echo "Failed to log in to ACR ${TARGET_ACR}: ${output}"
  exit 1
fi
TARGET_ACR_LOGIN_SERVER="$(jq --raw-output .loginServer <<<"${RESPONSE}" )"
oras login --registry-config "${AUTH_JSON}" \
           --username 00000000-0000-0000-0000-000000000000 \
           --password-stdin \
           "${TARGET_ACR_LOGIN_SERVER}" <<<"$( jq --raw-output .accessToken <<<"${RESPONSE}" )"

# at this point we have an auth config that can read from the source registry and
# write to the target registry.

# Check for DRY_RUN
if [[ "${DRY_RUN:-false}" == "true" ]]; then
    echo "DRY_RUN is enabled. Exiting without making changes."
    exit 0
fi

# mirror image
SRC_IMAGE="${SOURCE_REGISTRY}/${REPOSITORY}@${DIGEST}"
DIGEST_NO_PREFIX=${DIGEST#sha256:}
# we use the digest as a tag so the image can be inspected easily in the ACR
# this does not affect the fact that the image is stored by immutable digest in the ACR
# it is crucial though, that the tagged image is not used in favor of the @sha256:digest one
# as the tag is NOT guaranteed to be immutable
TARGET_IMAGE="${TARGET_ACR_LOGIN_SERVER}/${REPOSITORY}:${DIGEST_NO_PREFIX}"
echo "Mirroring image ${SRC_IMAGE} to ${TARGET_IMAGE}."
echo "The image will still be available under it's original digest ${DIGEST} in the target registry."
oras cp "${SRC_IMAGE}" "${TARGET_IMAGE}" --from-registry-config "${AUTH_JSON}" --to-registry-config "${AUTH_JSON}"
