# Provider Terraform Dimdam's

Gère la configuration des services existants (pare-feu, alertes, variables, clés SSH, tâches planifiées,
domaine personnel) via l'API REST. Documentation utilisateur : `content/api/terraform.md` du dépôt
`dimdams-docs` (docs.dimdams.com/api/terraform/).

## Développement

```bash
go test ./...
go vet ./...
```

Essai local sans publier : `go build -o ~/tf-dev/terraform-provider-dimdams .`, puis dans `~/.terraformrc` :

```hcl
provider_installation {
  dev_overrides { "dimdam-s/dimdams" = "/home/<vous>/tf-dev" }
  direct {}
}
```

`api_url = "http://127.0.0.1:3000"` est accepté (HTTP uniquement sur la boucle locale).

## Publier une version

1. Mettre à jour la version dans la documentation (`terraform.md`).
2. `bash scripts/build-terraform-provider.sh 1.1.0` : archives et `SHA256SUMS` dans `public/terraform/1.1.0/`,
   `public/terraform/latest.json` mis à jour. Garder les anciennes versions : les fichiers de verrouillage
   des clients les référencent.
3. Committer, déployer, purger le cache Cloudflare (`latest.json`).

## Registre public Terraform

Le registre ne lit que des dépôts GitHub **publics** nommés `terraform-provider-<nom>`, et l'espace de noms
est le compte GitHub qui possède le dépôt. Le nom `dimdams` étant pris sur GitHub, le dépôt est
`github.com/Dimdam-s/terraform-provider-dimdams` et le provider s'appelle `dimdam-s/dimdams` sur le registre.

Ce dossier contient déjà tout ce qu'il faut : `LICENSE` (MPL-2.0), `.goreleaser.yml`,
`terraform-registry-manifest.json`, `.github/workflows/release.yml` (version signée à chaque tag `v*`),
`.github/workflows/test.yml`, `examples/` et `docs/` au format du registre (générés par
`go generate ./...` avec tfplugindocs ; à relancer après tout changement de schéma).

Mise en place, une seule fois :

1. Créer le dépôt public vide `terraform-provider-dimdams` sous le compte ou l'organisation GitHub choisi.
2. Créer une clé GPG RSA ou DSA réservée aux versions (`gpg --full-generate-key`, sans expiration courte).
3. Dans le dépôt public, **Settings › Secrets › Actions** : `GPG_PRIVATE_KEY` (`gpg --armor --export-secret-keys <ID>`)
   et `PASSPHRASE`.
4. Sur [registry.terraform.io](https://registry.terraform.io), se connecter avec GitHub, **Publish › Provider**,
   ajouter la clé GPG **publique** (`gpg --armor --export <ID>`) dans les paramètres de l'espace de noms, puis choisir le dépôt.
5. Depuis ce dépôt : `bash scripts/publish-terraform-provider.sh git@github.com:Dimdam-s/terraform-provider-dimdams.git v1.0.1`.

Le script n'envoie que ce dossier (`git subtree split`), jamais le reste du panel. Ensuite, chaque nouvelle
version : committer ici, puis relancer le script avec le tag suivant.

Une fois sur le registre, `terraform init` suffit côté client. Le script d'installation `public/terraform/install.sh`
peut rester pour les machines sans accès au registre, mais un provider installé par lui est pris en priorité
sur le registre : le client doit supprimer `~/.terraform.d/plugins/registry.terraform.io/dimdam-s` pour passer au registre.

## Nouvelle ressource

Une ressource ne doit appeler que des routes qui acceptent les clés d'API (`apiOrSession` ou
`authenticateRequest` avec la permission attendue) et ne jamais rien écrire de secret dans les journaux.
