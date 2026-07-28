# Gowlarr

**⚠️ Disclaimer légal / non-affiliation**

Gowlarr est un outil personnel, développé de manière indépendante, qui n'est **ni
affilié, ni approuvé, ni sponsorisé** par le projet [Prowlarr](https://github.com/Prowlarr/Prowlarr),
[Servarr](https://wiki.servarr.com/) ou tout autre projet de l'écosystème *arr*.

Gowlarr est un outil d'automatisation neutre : il fournit un moyen technique de
rechercher et de résoudre des liens (torrent/magnet/nzb) sur des sites tiers que
**vous** configurez. Gowlarr ne stocke, n'héberge et ne distribue aucun contenu
protégé par le droit d'auteur. **Vous êtes seul responsable de l'usage que vous
faites de cet outil et de la légalité, dans votre juridiction, de son utilisation
avec les sites indexeurs que vous choisissez d'y connecter.**

Les définitions d'indexeurs (format Cardigann, YAML) ne sont **jamais redistribuées**
avec ce projet : elles sont téléchargées à l'exécution, à la demande de l'utilisateur,
depuis le dépôt public [Prowlarr/Indexers](https://github.com/Prowlarr/Indexers), qui
reste la propriété de ses auteurs respectifs.

## Statut

Projet en cours de développement (MVP). Voir `plan.md` (hors dépôt) pour la roadmap.

## Licence

Le code de ce projet est distribué sous licence MIT (voir `LICENSE`). Cette licence
ne s'applique qu'au code de Gowlarr lui-même, pas aux définitions Cardigann tierces
téléchargées à l'exécution. Le moteur Cardigann a été extrait dans le module Go
séparé `github.com/Kcchouette/cardigann-go`, distribué sous **LGPL-3.0** ; Gowlarr
l'utilise via un `replace ../cardigann-go` local tant que le module n'est pas publié.

## Installation / Build

```powershell
go build ./cmd/gowlarr
```

Le dépôt compagnon `github.com/Kcchouette/cardigann-go` porte les packages réutilisables
du moteur Cardigann (parsing YAML, templates, filtres, client HTTP, login, engine).

## Utilisation (MVP)

```powershell
gowlarr config init
gowlarr search "some query"
gowlarr download <result-id>
```
