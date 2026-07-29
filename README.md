# Gowlarr

**Disclaimer légal / non-affiliation**

Gowlarr est un outil personnel, développé de manière indépendante, qui n'est **ni
affilié, ni approuvé, ni sponsorisé** par le projet [Prowlarr](https://github.com/Prowlarr/Prowlarr),
[Servarr](https://wiki.servarr.com/) ou tout autre projet de l'écosystème \*arr.

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

## Installation

```bash
# Depuis les sources (Go 1.25+ requis)
go install github.com/Kcchouette/gowlarr/cmd/gowlarr@latest

# Ou depuis le dépôt cloné
git clone https://github.com/Kcchouette/gowlarr.git
cd gowlarr
go build ./cmd/gowlarr
```

## Utilisation

### Initialiser la configuration

```bash
gowlarr config init
```

Crée le fichier de configuration par défaut (`~/.config/gowlarr/config.json` ou
`%APPDATA%/gowlarr/config.json` sous Windows) et la base SQLite.

### Synchroniser les définitions d'indexeurs

```bash
gowlarr defs sync          # Télécharge les définitions Cardigann depuis GitHub
gowlarr defs list           # Liste les définitions disponibles
gowlarr defs show <id>      # Affiche le détail d'une définition
```

### Gérer les indexeurs

```bash
gowlarr indexer add <definition-id>           # Ajouter un indexeur
gowlarr indexer list                          # Lister les indexeurs configurés
gowlarr indexer test <id>                     # Tester la connexion
gowlarr indexer enable/disable <id>          # Activer/désactiver
gowlarr indexer remove <id>                   # Supprimer
```

### Rechercher

```bash
gowlarr search "ubuntu"                                  # Recherche simple
gowlarr search "ubuntu" --indexer 1337x                  # Indexeur spécifique
gowlarr search "ubuntu" --protocol torrent               # Filtrer par protocole
gowlarr search "ubuntu" --categories 2000,2010           # Filtrer par catégorie
gowlarr search "ubuntu" --json                           # Sortie JSON
```

### Télécharger

```bash
gowlarr download <result-id>                # Télécharger un résultat
gowlarr download <result-id> -o ./out.torrent  # Sauvegarder dans un fichier
gowlarr download <result-id> --stdout       # Afficher sur stdout
```

### Usenet (Newznab)

```bash
gowlarr search "query" --newznab-url https://indexer.example.com --newznab-apikey YOUR_KEY
```

## Commandes disponibles

| Commande | Description |
|----------|-------------|
| `config init` | Créer la configuration par défaut |
| `config show` | Afficher la configuration active |
| `defs sync` | Synchroniser les définitions depuis GitHub |
| `defs list` | Lister les définitions disponibles |
| `defs show <id>` | Afficher le détail d'une définition |
| `indexer add <id>` | Ajouter un indexeur |
| `indexer list` | Lister les indexeurs configurés |
| `indexer test <id>` | Tester la connexion d'un indexeur |
| `indexer enable/disable <id>` | Activer/désactiver un indexeur |
| `indexer remove <id>` | Supprimer un indexeur |
| `search <query>` | Rechercher sur les indexeurs |
| `download <id>` | Télécharger un résultat |

## Architecture

```
cmd/gowlarr/          Point d'entrée CLI
internal/cli/         Commandes Cobra
internal/config/      Gestion de la configuration
internal/store/       Persistance SQLite (migrations, indexeurs, résultats)
internal/model/       Types partagés (ReleaseInfo, Protocol)
internal/search/      Moteur de recherche (providers parallèles)
internal/download/    Résolution des liens (magnet, torrent, nzb)
internal/newznab/     Client Newznab natif
internal/cardigannadapter/  Adaptateur vers cardigann-go
```

## Dépendances

- [cardigann-go](https://github.com/Kcchouette/cardigann-go) — Moteur Cardigann (LGPL-3.0)
- [cobra](https://github.com/spf13/cobra) — Framework CLI
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — SQLite pur Go (sans cgo)

## Sécurité

> [!IMPORTANT]
> Les paramètres d'indexeur persistés en base SQLite (`settings_json`,
> y compris d'éventuels identifiants/mots de passe) sont actuellement stockés
> **en clair**. L'infrastructure de chiffrement existe bien dans `internal/crypt`,
> mais elle n'est pas encore branchée dans le flux normal d'exécution
> (la clé de chiffrement reste `nil`).

## Licence

MIT — voir [LICENSE](LICENSE). La licence ne s'applique qu'au code de Gowlarr lui-même,
pas aux définitions Cardigann tierces téléchargées à l'exécution.
