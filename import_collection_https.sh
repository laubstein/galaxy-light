#!/bin/bash
# Exemplo de execução:
# ./import_collection_https.sh ansible.utils
# ./import_collection_https.sh netbox.netbox netbox-community/ansible_modules

COLLECTION_NAME="$1"
COLLECTION_REPO="${2:-ansible-collections/$COLLECTION_NAME}"

GITLAB_USERNAME=${GITLAB_USERNAME:-root}
GITLAB_TOKEN=${GITLAB_TOKEN:-zDy4WEJ1bxToj7kadrV-}
GITLAB_INSTANCE=${GITLAB_INSTANCE:-http://127.0.0.1:8080}
GITLAB_COLLECTIONS_GROUP=${GITLAB_COLLECTIONS_GROUP:-ansible/collections}
GITLAB_ENDPOINT=$(echo -n "${GITLAB_INSTANCE}" | sed -r "s%(https?://)%\1${GITLAB_USERNAME}:${GITLAB_TOKEN}@%")

if [ -z "${COLLECTION_NAME}" ]
then
	echo "collection precisa ser passada"
	exit 1
fi

set -e
echo " ============================================================================ "
echo " _                            _               _ _           _   _             "
echo "(_)_ __ ___  _ __   ___  _ __| |_    ___ ___ | | | ___  ___| |_(_) ___  _ __  "
echo "| | '_ \` _ \| '_ \ / _ \| '__| __|  / __/ _ \| | |/ _ \/ __| __| |/ _ \| '_ \ "
echo "| | | | | | | |_) | (_) | |  | |_  | (_| (_) | | |  __/ (__| |_| | (_) | | | |"
echo "|_|_| |_| |_| .__/ \___/|_|   \__|  \___\___/|_|_|\___|\___|\__|_|\___/|_| |_|"
echo "            |_|                                                               "
echo " ============================================================================ "
echo " COLLECTION NAME : ${COLLECTION_NAME}"
echo " SOURCE          : https://github.com/${COLLECTION_REPO}.git"
echo " TARGET          : ${GITLAB_INSTANCE}/${GITLAB_COLLECTIONS_GROUP}/${COLLECTION_NAME}"
echo " ============================================================================ "
git clone --mirror https://github.com/${COLLECTION_REPO}.git ${COLLECTION_NAME}/.git
cd "${COLLECTION_NAME}"
git config --unset core.bare
git remote add internal ${GITLAB_ENDPOINT}/${GITLAB_COLLECTIONS_GROUP}/${COLLECTION_NAME}.git
# corrige repositórios com tags "vX.Y.Z" e cria a tag "X.Y.Z"
git tag | grep 'v[0-9]' | tr -d 'v' | while read TAG
do
    echo "FIX TAG v${TAG} => ${TAG}"
    git tag "${TAG}" "v${TAG}"
done
git tag | grep '\.0[0-9]' | while read TAG
do
    FIXED_TAG=$(echo -n "${TAG}" | sed -r 's/[.]0+([0-9]+)/.\1/g;s/^0+([0-9]+)/\1/g')
    echo "FIX TAG ${TAG} => ${FIXED_TAG}"
    git tag "${FIXED_TAG}" "${TAG}"
done
git push internal --tags
git push internal --all
cd -
rm -fr "${COLLECTION_NAME}"

if [ -z "$GITLAB_TOKEN" ]
then
    echo "GITLAB_TOKEN não encontrado, pulando configuração do projeto"
else
    echo "GITLAB_TOKEN encontrado, atualizando configurações do repositório"
    PROJECT_ID=$(printf %s "${GITLAB_COLLECTIONS_GROUP}/${COLLECTION_NAME}" | jq -Rr @uri)
    UPDATED_AT=$(date '+%Y-%m-%d %H:%m')
    curl -sk --request PUT --header "PRIVATE-TOKEN: ${GITLAB_TOKEN}" \
        --url "${GITLAB_INSTANCE}/api/v4/projects/${PROJECT_ID}" \
        --data "visibility=public" \
        --data "description=https://github.com/${COLLECTION_REPO} atualizado em ${UPDATED_AT}" \
        --data "topics[]=ansible" \
        --data "topics[]=collections" \
        --data "topics[]=${COLLECTION_NAME}" -o /dev/null
fi
