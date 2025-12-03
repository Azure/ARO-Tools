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

    # Check if source registry is an OpenShift CI registry https://docs.ci.openshift.org/docs/how-tos/use-registries-in-build-farm/#summary-of-available-registries
    CI_REGISTRIES=(
        "registry.build01.ci.openshift.org"
        "registry.build02.ci.openshift.org"
        "registry.build03.ci.openshift.org"
        "registry.build04.ci.openshift.org"
        "registry.build05.ci.openshift.org"
        "registry.build06.ci.openshift.org"
        "registry.build07.ci.openshift.org"
        "registry.build08.ci.openshift.org"
        "registry.build09.ci.openshift.org"
        "registry.build10.ci.openshift.org"
        "registry.build11.ci.openshift.org"
        "registry.core.ci.openshift.org"
        "test-disruption-ct59n-openshift-image-registry.apps.build02.vmc.ci.openshift.org"
    )

    IS_CI_REGISTRY=false
    for CI_REG in "${CI_REGISTRIES[@]}"; do
        if [[ "${SOURCE_REGISTRY}" == "${CI_REG}" ]]; then
            IS_CI_REGISTRY=true
            break
        fi
    done

    if [[ "${IS_CI_REGISTRY}" == "true" ]]; then
        echo "Setting up registry authentication for CI source registry."
        cp ~/.docker/config.json "${AUTH_JSON}" 2>/dev/null || echo '{}' > "${AUTH_JSON}"
        oc registry login --to "${AUTH_JSON}"
    else
        echo "Fetch pull secret for source registry ${SOURCE_REGISTRY} from ${PULL_SECRET_KV} KV."
        az keyvault secret download --vault-name "${PULL_SECRET_KV}" --name "${PULL_SECRET}" -e base64 --file "${AUTH_JSON}"
    fi

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
    REQUIRED_VARS=("TARGET_ACR" "REPOSITORY" "IMAGE_TAR_FILE_NAME" "IMAGE_METADATA_FILE_NAME")
    for VAR in "${REQUIRED_VARS[@]}"; do
        if [ -z "${!VAR}" ]; then
            echo "Error: Environment variable $VAR is not set."
            exit 1
        fi
    done

    # set image file path to pwd if not set
    if [ -z "${IMAGE_FILE_PATH}" ]; then
        IMAGE_FILE_PATH="$(pwd)"
    fi

    IMAGE_TAR_FILE="${IMAGE_FILE_PATH}/${IMAGE_TAR_FILE_NAME}"
    # validate image tar file exists
    if [ ! -f "${IMAGE_TAR_FILE}" ]; then
        echo "Error: Image tar file ${IMAGE_TAR_FILE_NAME} does not exist at a given path ${IMAGE_FILE_PATH}."
        exit 1
    fi

    IMAGE_METADATA_FILE="${IMAGE_FILE_PATH}/${IMAGE_METADATA_FILE_NAME}"
    # validate image metadata file exists
    if [ ! -f "${IMAGE_METADATA_FILE}" ]; then
        echo "Error: Image metadata file ${IMAGE_METADATA_FILE_NAME} does not exist at a given path ${IMAGE_FILE_PATH}."
        exit 1
    fi

    # Extract build_tag using jq
    BUILD_TAG=$(jq -r '.build_tag' "$IMAGE_METADATA_FILE")

    if [[ -z "$BUILD_TAG" || "$BUILD_TAG" == "null" ]]; then
    echo "❌ build_tag not found in $IMAGE_METADATA_FILE" >&2
    exit 1
    fi

    echo "✅ build_tag is: $BUILD_TAG"    

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
    TARGET_IMAGE="${TARGET_ACR_LOGIN_SERVER}/${REPOSITORY}:${BUILD_TAG}"
    oras cp --from-oci-layout "${IMAGE_TAR_FILE}:${BUILD_TAG}" "${TARGET_IMAGE}"
}

if [[ -z "${IMAGE_TAR_FILE_NAME:-}" ]]; then
    copyImageFromRegistry
else
    copyImageFromOciLayout
fi