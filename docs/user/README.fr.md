# Karyora — Guide de l'utilisateur

Ce guide décrit l'application de bureau Karyora telle qu'elle est livrée aujourd'hui. Il énonce uniquement ce que le produit fait réellement ; les capacités qui n'existent pas encore sont listées comme limites et ne sont jamais présentées comme des promesses. Ce même guide est disponible en 12 langues dans ce dossier.

## 1. Ce qu'est Karyora

- Karyora est une application de bureau à priorité locale (local-first). Vos packs de connaissances, vos travaux et vos réglages sont stockés sur votre propre machine.
- Le produit fonctionne sans aucun service en ligne du projet. Le seul trafic réseau qu'il peut générer est celui que vous déclenchez : des requêtes vers vos fournisseurs d'IA, un import de pack de connaissances que vous demandez et — si vous le configurez — le canal de mises à jour.
- Les fonctions d'IA utilisent vos propres comptes fournisseurs (BYOK, « bring your own key »). Vous configurez vos propres clés d'API, et le fournisseur vous facture directement. Karyora ne revend jamais l'usage des fournisseurs et ne sert jamais d'intermédiaire pour leurs paiements.

## 2. Versions communautaires et versions officielles

- Le code source de Karyora est publié sous la licence Apache License 2.0. Vous pouvez l'utiliser, l'étudier, le modifier et le redistribuer selon les termes de cette licence, y compris le compiler vous-même.
- Le dépôt public est `https://github.com/qira-dev/karyora`. Chaque version est marquée par une étiquette `v*`, et l'arborescence source de cette étiquette constitue le code source de la version.
- Une version « officielle » est une version construite par le pipeline de build du projet lui-même, à une révision étiquetée, avec des preuves enregistrées. Les builds officiels et communautaires ont les mêmes fonctions ; la différence réside dans la personne qui a construit l'artefact et celle qui en répond, jamais dans ce que l'artefact sait faire.
- En v1, il n'existe encore ni page officielle de téléchargement ni canal officiel de mises à jour. En attendant leur publication, vous pouvez vous procurer Karyora depuis le dépôt public et le compiler depuis les sources.

## 3. Plateformes prises en charge (v1)

| OS | CPU | Plancher |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 ou plus récent |
| Windows 11 | ARM64 | Windows 11 pour ARM |
| macOS | x86_64 | macOS 15 (Sequoia) ou plus récent |
| macOS | ARM64 | macOS 15 (Sequoia) ou plus récent, Apple Silicon |
| Linux | x86_64 | glibc 2.34 ou plus récent ; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 ou plus récent ; les mêmes familles de distributions |

Explicitement non pris en charge en v1 : Windows 10 ; macOS 14 ou plus ancien ; les distributions basées sur musl comme Alpine ; toute combinaison non listée ci-dessus. La prise en charge suit la fenêtre de sécurité de chaque fournisseur et est revérifiée à chaque version.

## 4. Paquets et vérification

- L'ensemble de paquets produits est : MSI Windows, pkg macOS, deb Linux, rpm Linux et une archive portable Linux — un artefact par architecture CPU prise en charge.
- Chaque paquet est produit avec un manifeste `SHA256SUMS` ainsi que des enregistrements SBOM et de provenance par artefact. Pour vérifier un paquet : calculez l'empreinte sha256 de votre téléchargement et comparez-la à l'entrée du manifeste. Si l'empreinte ne correspond pas, n'installez pas le paquet.
- Les paquets ne sont pas signés numériquement en v1 — par conception. Les étapes de vérification de signature au niveau de la plateforme (vérifications de signature du système d'exploitation, clés de dépôt) seront documentées avec la première version signée. D'ici là, la comparaison d'empreintes ci-dessus est la vérification à votre disposition.
- Les fichiers `LICENSE` et `NOTICE`, qui portent la licence du projet et les mentions de tiers, font partie du dépôt source.

## 5. Mises à jour

- Karyora contient un mécanisme de mise à jour, livré désactivé par défaut en v1. Aucun canal de mise à jour n'est configuré : une vérification de mise à jour renvoie donc un refus typé indiquant qu'aucun point de terminaison n'est configuré. C'est voulu : le canal officiel de mises à jour ne sera activé qu'une fois publié.
- D'ici là, mettre à jour signifie installer le paquet plus récent par-dessus l'ancien. Vos données sont préservées lors des mises à jour (voir la section 6).

## 6. Vos données

- Toutes les données du produit résident sur votre machine. Le répertoire de données par défaut est `%APPDATA%\Karyora\data` sous Windows, `~/Library/Application Support/Karyora/data` sous macOS et `~/.local/share/karyora/data` sous Linux (le réglage XDG `XDG_DATA_HOME` est respecté). Vous pouvez choisir un autre emplacement avec l'option `--data-dir`.
- Le produit n'effectue pas de sauvegardes planifiées. Pour sauvegarder : arrêtez le produit puis copiez le répertoire de données. Restaurer consiste à remettre le répertoire copié en place. Il n'y a pas de sauvegarde en ligne.
- Désinstaller ou mettre à jour ne supprime pas vos données. La suppression des données est une action distincte et explicite que vous effectuez vous-même.
- Les données au repos ne sont pas chiffrées. Protégez le répertoire de données par vos propres moyens, par exemple le chiffrement complet du disque. C'est une limite connue, énoncée franchement.
- Les identifiants des fournisseurs (vos clés d'API) sont stockés uniquement dans le trousseau de votre système d'exploitation. Ils ne sont jamais écrits dans des fichiers, des journaux ou le contenu des requêtes.

## 7. Confidentialité

- Le produit ne comporte aucune télémétrie : rien n'est envoyé aux serveurs du projet.
- Les diagnostics sont produits localement et préservent la confidentialité par défaut ; c'est vous qui décidez si quelque chose quitte votre machine.

## 8. Limites connues en v1

- L'exécution des plugins tiers est désactivée.
- Le fonctionnement en équipe et à distance n'est pas disponible.
- Transférer un travail actif d'une machine à une autre n'est pas pris en charge.
- La synchronisation des packs de connaissances portables n'est pas disponible.
- La fonction de regroupement « Work » n'existe pas.
- Il n'y a ni abonnements payants ni droits (entitlements) : rien à acheter, aucune offre payante.
- Aucune promesse de performance chiffrée n'est faite.
- Les données au repos ne sont pas chiffrées (voir la section 6).
- Il n'y a pas encore de page officielle de téléchargement ni de canal de mises à jour (voir les sections 2 et 5).

## 9. Licence

- Karyora est publié sous la licence Apache License 2.0. Le texte complet de la licence se trouve dans le fichier `LICENSE` et les mentions de tiers dans le fichier `NOTICE`, tous deux dans le dépôt source.
- Compiler Karyora vous-même, en créer un fork ou le redistribuer selon la licence conserve tous les droits que la licence accorde. Ce droit ne dépend d'aucun achat, puisqu'il n'y a rien à acheter.

## 10. Référence en ligne de commande

L'outil en ligne de commande est `uawp`.

| Commande | Effet |
|---|---|
| `uawp start` | Démarre le service local. Un second démarrage sur le même répertoire de données est refusé tant qu'une instance est en cours. |
| `uawp stop` | Arrête le service avec un arrêt borné et ordonné. |
| `uawp status` | Indique si le service est en cours d'exécution. |
| `uawp doctor` | Exécute des vérifications locales d'état et de diagnostic. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Vérifie, applique ou rapporte les mises à jour. En v1, sans canal configuré, check et apply se terminent par un refus typé indiquant qu'aucun point de terminaison n'est configuré ; `status` rapporte l'état des mises à jour. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Soumet un travail durable, rapporte son statut ou suit ses événements. |
| `uawp credential store --provider <id> [--name api-key]` | Enregistre votre clé d'API du fournisseur dans le trousseau du système d'exploitation. |
| `uawp credential status` | Indique si un identifiant existe — jamais sa valeur. |
| `uawp credential remove` | Supprime l'identifiant enregistré. |
| `uawp version` / `uawp help` | Affiche la version ou l'aide. |

Un identifiant enregistré n'ouvre rien par lui-même : un véritable appel au fournisseur doit encore passer les règles de traitement des données que le produit applique à ce fournisseur.
