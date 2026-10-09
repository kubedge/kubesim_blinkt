"""Which clone am I in, and what is it — `project_root` / `meta_root` / stack detection.

Every verb answered this for itself, and they disagreed: one tested the directory
name, another the origin URL, a third both. A verb that guesses wrong writes into the
wrong repository, which is what Guardrail 4 exists to prevent.
"""

from __future__ import annotations

from pathlib import Path

from . import git
from .io import die

STACK_MARKERS = {"go": "go.mod", "python": "pyproject.toml", "node": "package.json"}
META_REPO_NAME = "claude-meta"


def toplevel(start: Path | None = None) -> Path:
    """The git root containing `start` (default: cwd). Exits if there is none."""
    top = git.probe(start or Path.cwd(), "rev-parse", "--show-toplevel")
    if not top:
        die("not a git repository")
    return Path(top)


def origin_url(root: Path) -> str:
    """The origin remote, or "" when there is none — a fresh clone is not an error here."""
    return git.probe(root, "remote", "get-url", "origin") or ""


def is_meta_repo(root: Path) -> bool:
    """Is this a claude-meta clone, canonical or fork?

    Both conditions, deliberately: a project may be *named* claude-meta-something, and
    a meta clone may sit in a renamed directory. Requiring both was the behaviour that
    survived review; either alone produced a false positive in the fleet.
    """
    return META_REPO_NAME in origin_url(root) and root.name == META_REPO_NAME


def is_canonical(root: Path) -> bool:
    """Canonical has no `upstream` remote; a fork does. The manifests follow from this."""
    return is_meta_repo(root) and git.probe(root, "remote", "get-url", "upstream") is None


def stacks(root: Path) -> list[str]:
    """Which toolchains this repo actually has, by marker file."""
    return sorted(name for name, marker in STACK_MARKERS.items() if (root / marker).is_file())


def require_project(root: Path | None = None) -> Path:
    """The project root, refusing on a meta clone.

    Guardrail 4: meta broadcasts, it never applies a delivery into a project — not even
    by driving the project's own verb from the meta side.
    """
    root = root or toplevel()
    if is_meta_repo(root):
        die(
            "this is the claude-meta clone — the consumer half runs in the PROJECT's own "
            "session (Guardrail 4). Meta stages; it never applies."
        )
    return root


def require_meta(root: Path | None = None) -> Path:
    """The meta root, refusing anywhere else."""
    root = root or toplevel()
    if not is_meta_repo(root):
        die("this verb runs in a claude-meta clone; this is not one")
    return root
