#!/bin/bash

set -euo pipefail

copyImageFromRegistry() {
    # shortcut mirroring if the source registry is the same as the target ACR
    REQUIRED_REGISTRY_VARS=("TARGET_ACR" "SOURCE_REGISTRY")
    for VAR in "${REQUIRED_REGISTRY_VARS[@]}"; do
        if [ -z "${!VAR}" ]; then
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
    REQUIRED_VARS=("PULL_SECRET_KV" "PULL_SECRET" "REPOSITORY" "DIGEST")
    for VAR in "${REQUIRED_VARS[@]}"; do
        if [ -z "${!VAR}" ]; then
            echo "Error: Environment variable $VAR is not set."
            exit 1
        fi
    done

    # create temporary FS structure
    TMP_DIR="$(mktemp -d)"
    CONTAINERS_DIR="${TMP_DIR}/containers"
    AUTH_JSON="${CONTAINERS_DIR}/auth.json"
    ORAS_CACHE="${TMP_DIR}/oras-cache"
    mkdir -p "${CONTAINERS_DIR}"
    mkdir -p "${ORAS_CACHE}"
    trap 'rm -rf ${TMP_DIR}' EXIT

    # get pull secret for source registry
    echo "Fetch pull secret for source registry ${SOURCE_REGISTRY} from ${PULL_SECRET_KV} KV."
    az keyvault secret download --vault-name "${PULL_SECRET_KV}" --name "${PULL_SECRET}" -e base64 --file "${AUTH_JSON}"

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
    if [ "${DRY_RUN:-false}" == "true" ]; then
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
}

copyImageFromOciLayout() {
    # validate required variables
    REQUIRED_VARS=("TARGET_ACR" "REPOSITORY" "DIGEST" "OCI_LAYOUT_PATH")
    for VAR in "${REQUIRED_VARS[@]}"; do
        if [ -z "${!VAR}" ]; then
            echo "Error: Environment variable $VAR is not set."
            exit 1
        fi
    done

    # validate OCI layout path exists
    if [ ! -d "${OCI_LAYOUT_PATH}" ]; then
        echo "Error: OCI layout directory ${OCI_LAYOUT_PATH} does not exist."
        exit 1
    fi

    ACR_DOMAIN_SUFFIX="$(az cloud show --query "suffixes.acrLoginServerEndpoint" --output tsv)"
    TARGET_ACR_LOGIN_SERVER="${TARGET_ACR}${ACR_DOMAIN_SUFFIX}"

    echo "Getting the ACR access token."
    USERNAME="00000000-0000-0000-0000-000000000000"
    PASSWORD=$(az acr login --name "$TARGET_ACR" --expose-token --output tsv --query accessToken)
 
    echo "Logging in with ORAS."
    oras login $TARGET_ACR_LOGIN_SERVER --username $USERNAME  --password-stdin <<< $PASSWORD
   
    # Check for DRY_RUN
    if [ "${DRY_RUN:-false}" == "true" ]; then
        echo "DRY_RUN is enabled. Exiting without making changes."
        exit 0
    fi

    # copy image from OCI layout to ACR
    DIGEST_NO_PREFIX=${DIGEST#sha256:}
    TARGET_IMAGE="${TARGET_ACR_LOGIN_SERVER}/${REPOSITORY}:${DIGEST_NO_PREFIX}"

    echo "Copying image from OCI layout ${OCI_LAYOUT_PATH} to ${TARGET_IMAGE}."
    oras cp --from-oci-layout "${OCI_LAYOUT_PATH}@${DIGEST}" "${TARGET_IMAGE}"
}

if [[ -z "${OCI_LAYOUT_PATH:-}" ]]; then
    copyImageFromRegistry
else
    copyImageFromOciLayout
fi