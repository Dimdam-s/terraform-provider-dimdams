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
  dev_overrides { "dimdams/dimdams" = "/home/<vous>/tf-dev" }
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

## Nouvelle ressource

Une ressource ne doit appeler que des routes qui acceptent les clés d'API (`apiOrSession` ou
`authenticateRequest` avec la permission attendue) et ne jamais rien écrire de secret dans les journaux.
