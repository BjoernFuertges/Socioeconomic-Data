"""
Vergleich und Evaluation von vier Imputationsmethoden für fehlende Werte in
den Kreis-Zeitreihen (results_*_kreise, geodata_id_nuts als Regionsschlüssel):

  1. Lineare Interpolation zwischen Vor- und Nachjahr (gleiche Region/Dataset).
  2. Naive Fortschreibung: der letzte bekannte Vorjahreswert der Zielregion
     wird unverändert übernommen (kein Trend, keine Nachbarregionen) - dient
     als triviale Baseline, gegen die sich die aufwendigeren Methoden 3/4
     rechtfertigen müssen.
  3. Regionale/Peer-basierte Fortschreibung: durchschnittliche relative
     Änderung der Nachbarregionen (gleiche NUTS-2-Region) von Vorjahr auf
     Zieljahr, angewandt auf den bekannten Vorjahreswert der Zielregion.
  4. Regressionsbasierte Schätzung: Mehrfache lineare Regression (OLS via
     numpy.linalg.lstsq) über die anderen 12 sozioökonomischen Kreis-
     Indikatoren im Zieljahr, angewandt auf die bekannten Kovariaten der
     Zielregion.

Zwei Betriebsarten:

  a) Demo der zwei tatsächlich gefundenen Lücken (Kelheim/wohngeldhaushalte/
     2021, Neitersen/beschaeftigungsquote_gemeinden/2020) - siehe
     `demo_known_gaps()`.

  b) Systematische Evaluation (Leave-one-out): Für eine Stichprobe echter,
     tatsächlich bekannter Werte wird der Wert künstlich "versteckt", mit
     allen drei Methoden geschätzt und die Abweichung zum echten Wert
     gemessen (MAE, RMSE, MAPE, Bias, Trefferquote) - siehe `run_evaluation()`.
     Nur für die 13 Kreis-Datensätze möglich (Gemeinden-Datensatz hat
     keinen NUTS-3-Code für Peer-Gruppierung/Regression).

Voraussetzung: lokale Postgres/PostGIS-DB mit geladenen Rohdaten im
loaded_data-Schema (siehe pipeline/dlt/run_all_pipelines.sh für den Ladevorgang).
Für eine rein lokale Ausführung außerhalb von docker-compose:
  - Postgres/PostGIS-Container z.B. mit
    `docker run -d --name sozio_postgis -e POSTGRES_DB=soziooekonomische-daten
     -e POSTGRES_USER=sozio_data_compose_user
     -e POSTGRES_PASSWORD=sozio_data_compose_user -p 5432:5432 postgis/postgis:17-3.5`
  - Verbindungsdaten können per PGHOST/PGPORT/PGDATABASE/PGUSER/PGPASSWORD
    überschrieben werden (Defaults siehe unten passen zu obigem Container).

Aufruf:
    source pipeline/dlt/.venv/bin/activate
    python analysis/data-analysis/impute_missing_values.py
"""

from __future__ import annotations

import os
import warnings
from typing import Any, cast

import numpy as np
import pandas as pd
import psycopg2

# pandas.read_sql gibt bei einer rohen psycopg2-Connection (statt SQLAlchemy
# Engine) eine harmlose UserWarning aus - hier bewusst unterdrückt.
warnings.filterwarnings(
    "ignore", message="pandas only supports SQLAlchemy connectable"
)

PGHOST = os.environ.get("PGHOST", "localhost")
PGPORT = int(os.environ.get("PGPORT", "5432"))
PGDATABASE = os.environ.get("PGDATABASE", "soziooekonomische-daten")
PGUSER = os.environ.get("PGUSER", "sozio_data_compose_user")
PGPASSWORD = os.environ.get("PGPASSWORD", "sozio_data_compose_user")

# Kreis-Ebene-Datensätze (haben alle geodata_id_nuts) - Grundlage sowohl für
# die Regressions-Kovariaten als auch für die systematische Evaluation.
KREISE_TABLES = {
    "arbeitslose": "results_arbeitslose_kreise",
    "arbeitsvolumen": "results_arbeitsvolumen_kreise",
    "auszubildende": "results_auszubildende_kreise",
    "bevoelkerung": "results_bevoelkerung_kreise",
    "bruttoverdienst": "results_bruttoverdienst_kreise",
    "durchschnittsalter": "results_durchschnittsalter_kreise",
    "erwerbstaetige": "results_erwerbstaetige_pro_1000_einwohner_kreise",
    "kinderarmut": "results_kinderarmut_kreise",
    "medianeinkommen": "results_medianeinkommen_kreise",
    "schuldnerquote": "results_schuldnerquote_kreise",
    "sgb_ii_quote": "results_sgb_ii_quote_kreise",
    "studierende": "results_studierende_kreise",
    "wohngeldhaushalte": "results_wohngeldhaushalte_kreise",
}

NUTS2_PREFIX_LEN = 4  # z.B. "DE22" aus "DE226"


# ---------------------------------------------------------------------------
# Datenhaltung: einmal laden, als Pivot-Tabellen (Region x Jahr) im Speicher
# ---------------------------------------------------------------------------


def load_pivots(conn: psycopg2.extensions.connection) -> dict[str, pd.DataFrame]:
    """Lädt alle Kreis-Datensätze einmal und pivotiert sie zu Region x Jahr."""
    pivots = {}
    for slug, table in KREISE_TABLES.items():
        df = pd.read_sql(
            f"select geodata_id_nuts, year, value from loaded_data.{table}",
            conn,
        )
        pivots[slug] = df.pivot_table(
            index="geodata_id_nuts", columns="year", values="value", aggfunc="first"
        )
    return pivots


# ---------------------------------------------------------------------------
# Methode 1: Lineare Interpolation
# ---------------------------------------------------------------------------


def method_1_linear(pivots: dict[str, pd.DataFrame], slug: str, region: str, gap_year: int) -> float | None:
    wide = pivots[slug]
    if region not in wide.index:
        return None
    row = cast(pd.Series, wide.loc[region]).dropna()
    before = row[row.index < gap_year]
    after = row[row.index > gap_year]
    if before.empty or after.empty:
        return None
    y0, v0 = before.index[-1], before.iloc[-1]
    y1, v1 = after.index[0], after.iloc[0]
    return float(v0 + (v1 - v0) * (gap_year - y0) / (y1 - y0))


# ---------------------------------------------------------------------------
# Methode 2: Naive Fortschreibung (letzter bekannter Vorjahreswert)
# ---------------------------------------------------------------------------


def method_2_naive_carry_forward(pivots: dict[str, pd.DataFrame], slug: str, region: str, gap_year: int) -> float | None:
    """Letzter bekannter Wert vor gap_year, unverändert übernommen (LOCF)."""
    wide = pivots[slug]
    if region not in wide.index:
        return None
    row = cast(pd.Series, wide.loc[region]).dropna()
    before = row[row.index < gap_year]
    if before.empty:
        return None
    return float(before.iloc[-1])


# ---------------------------------------------------------------------------
# Methode 3: Regionale/Peer-basierte Fortschreibung
# ---------------------------------------------------------------------------


def method_3_peer_trend(
    pivots: dict[str, pd.DataFrame], slug: str, region: str, gap_year: int
) -> float | None:
    wide = pivots[slug]
    prev_year = gap_year - 1
    if region not in wide.index or prev_year not in wide.columns or gap_year not in wide.columns:
        return None
    own_prev_raw = wide.at[region, prev_year]
    if pd.isna(own_prev_raw):
        return None
    own_prev = float(cast(Any, own_prev_raw))

    prefix = region[:NUTS2_PREFIX_LEN]
    peer_regions = [r for r in wide.index if r.startswith(prefix) and r != region]
    if not peer_regions:
        return None

    sub = cast(pd.DataFrame, wide.loc[peer_regions, [prev_year, gap_year]]).dropna()
    # Peers mit Vorjahreswert 0 würden eine Division durch 0 (inf/nan)
    # verursachen und die relative Änderung unbrauchbar machen - ausschließen.
    sub = sub[sub[prev_year] != 0]
    if sub.empty:
        return None

    rel_change = (sub[gap_year] - sub[prev_year]) / sub[prev_year]
    avg_rel_change = rel_change.mean()
    if not np.isfinite(avg_rel_change):
        return None

    estimate = own_prev * (1 + avg_rel_change)
    return float(estimate) if np.isfinite(estimate) else None


# ---------------------------------------------------------------------------
# Methode 4: Regressionsbasierte Schätzung (OLS über andere Indikatoren)
# ---------------------------------------------------------------------------


def _build_cross_section(pivots: dict[str, pd.DataFrame], target_slug: str, year: int) -> pd.DataFrame | None:
    """Region x [andere 12 Indikatoren, target] für ein gegebenes Jahr."""
    predictor_slugs = [s for s in KREISE_TABLES if s != target_slug]
    cols = {}
    for s in predictor_slugs:
        wide = pivots[s]
        if year in wide.columns:
            cols[s] = wide[year]
    if not cols:
        return None
    X = pd.DataFrame(cols)
    target_wide = pivots[target_slug]
    if year not in target_wide.columns:
        return None
    merged = X.join(target_wide[year].rename("target"))
    return merged


def method_4_regression(
    cross_section_cache: dict[tuple[str, int], pd.DataFrame | None],
    pivots: dict[str, pd.DataFrame],
    target_slug: str,
    region: str,
    gap_year: int,
    exclude_region: str | None = None,
) -> tuple[float | None, float | None]:
    """
    Trainiert OLS auf allen Regionen mit vollständigen Daten im Zieljahr
    (Zielregion ausgeschlossen) und sagt den Wert der Zielregion voraus.
    Gibt (Schätzwert, In-Sample-R^2) zurück.
    """
    key = (target_slug, gap_year)
    if key not in cross_section_cache:
        cross_section_cache[key] = _build_cross_section(pivots, target_slug, gap_year)
    merged = cross_section_cache[key]
    if merged is None:
        return None, None

    exclude = exclude_region if exclude_region is not None else region
    train = merged.drop(index=exclude, errors="ignore").dropna()
    predictor_cols = [str(c) for c in merged.columns if c != "target"]

    if region not in merged.index:
        return None, None
    target_row = cast(pd.Series, merged.loc[[region], predictor_cols].iloc[0])
    if target_row.isna().any():
        return None, None
    if len(train) < len(predictor_cols) + 5:
        return None, None

    X_train = train[predictor_cols].to_numpy(dtype=float)
    y_train = train["target"].to_numpy(dtype=float)
    X_design = np.column_stack([np.ones(len(X_train)), X_train])

    coefs, *_ = np.linalg.lstsq(X_design, y_train, rcond=None)

    y_pred_train = X_design @ coefs
    ss_res = float(np.sum((y_train - y_pred_train) ** 2))
    ss_tot = float(np.sum((y_train - y_train.mean()) ** 2))
    r2 = 1 - ss_res / ss_tot if ss_tot > 0 else float("nan")

    x_target = np.concatenate([[1.0], target_row.to_numpy(dtype=float)])
    estimate = float(x_target @ coefs)
    return estimate, r2


# ---------------------------------------------------------------------------
# a) Demo der zwei tatsächlich gefundenen Lücken
# ---------------------------------------------------------------------------


def demo_known_gaps(conn: psycopg2.extensions.connection, pivots: dict[str, pd.DataFrame]) -> pd.DataFrame:
    print("#" * 78)
    print("# Demo: tatsächlich gefundene Lücken")
    print("#" * 78)

    rows = []

    # Kelheim - kann komplett aus den bereits geladenen Pivots berechnet werden
    slug, region, year = "wohngeldhaushalte", "DE226", 2021
    m1 = method_1_linear(pivots, slug, region, year)
    m2 = method_2_naive_carry_forward(pivots, slug, region, year)
    m3 = method_3_peer_trend(pivots, slug, region, year)
    cache: dict[tuple[str, int], pd.DataFrame | None] = {}
    m4, r2 = method_4_regression(cache, pivots, slug, region, year)
    print(f"\nKelheim (DE226) / {slug} / {year}:")
    print(f"  1. Lineare Interpolation:        {m1:.2f}" if m1 is not None else "  1. nicht möglich")
    print(f"  2. Naive Fortschreibung:         {m2:.2f}" if m2 is not None else "  2. nicht möglich")
    print(f"  3. Peer-basierte Fortschreibung: {m3:.2f}" if m3 is not None else "  3. nicht möglich")
    print(f"  4. Regressionsschätzung:         {m4:.2f}  (R²={r2:.3f})" if m4 is not None else "  4. nicht möglich")
    rows.append(
        {
            "dataset": slug, "region": region, "region_name": "Kelheim", "year": year,
            "est_1_linear": m1, "est_2_naive": m2, "est_3_peer": m3, "est_4_regression": m4, "r2_4": r2,
        }
    )

    # Neitersen - eigener Datensatz ohne NUTS-Code, nur Methode 1 anwendbar
    df = pd.read_sql(
        """select year, value from loaded_data.results_beschaeftigungsquote_gemeinden
           where geodata_name = %s order by year""",
        conn,
        params=("Neitersen",),
    )
    before = df[df["year"] < 2020]
    after = df[df["year"] > 2020]
    m1_neitersen = None
    m2_neitersen = None
    if not before.empty:
        m2_neitersen = float(before.iloc[-1]["value"])
    if not before.empty and not after.empty:
        y0, v0 = before.iloc[-1][["year", "value"]]
        y1, v1 = after.iloc[0][["year", "value"]]
        m1_neitersen = float(v0 + (v1 - v0) * (2020 - y0) / (y1 - y0))
    print("\nNeitersen / beschaeftigungsquote_gemeinden / 2020:")
    print(f"  1. Lineare Interpolation:        {m1_neitersen:.2f}" if m1_neitersen else "  1. nicht möglich")
    print(f"  2. Naive Fortschreibung:         {m2_neitersen:.2f}" if m2_neitersen else "  2. nicht möglich")
    print("  3. Peer-basierte Fortschreibung: nicht möglich (kein NUTS-Code im Gemeinden-Datensatz)")
    print("  4. Regressionsschätzung:         nicht möglich (kein NUTS-Code im Gemeinden-Datensatz)")
    print()
    rows.append(
        {
            "dataset": "beschaeftigungsquote_gemeinden", "region": "Neitersen",
            "region_name": "Neitersen", "year": 2020,
            "est_1_linear": m1_neitersen, "est_2_naive": m2_neitersen,
            "est_3_peer": None, "est_4_regression": None, "r2_4": None,
        }
    )

    return pd.DataFrame(rows)


# ---------------------------------------------------------------------------
# b) Systematische Evaluation (Leave-one-out über eine Stichprobe echter Werte)
# ---------------------------------------------------------------------------


def find_interior_candidates(wide: pd.DataFrame) -> list[tuple[str, int]]:
    """
    Alle (Region, Jahr)-Kombinationen mit bekanntem Wert, bei denen sowohl ein
    früheres als auch ein späteres bekanntes Jahr existiert (= Methode 1
    wäre grundsätzlich anwendbar, notwendig für einen fairen Vergleich).
    """
    candidates = []
    for region, row in wide.iterrows():
        known_years = row.dropna().index.to_numpy()
        if len(known_years) < 3:
            continue
        for i in range(1, len(known_years) - 1):
            candidates.append((str(region), int(known_years[i])))
    return candidates


def sample_holdouts(
    pivots: dict[str, pd.DataFrame], n_per_dataset: int, seed: int = 42
) -> pd.DataFrame:
    rng = np.random.default_rng(seed)
    rows = []
    for slug, wide in pivots.items():
        candidates = find_interior_candidates(wide)
        if not candidates:
            continue
        n = min(n_per_dataset, len(candidates))
        idx = rng.choice(len(candidates), size=n, replace=False)
        for i in idx:
            region, year = candidates[i]
            actual = wide.at[region, year]
            rows.append({"dataset": slug, "region": region, "year": year, "actual": actual})
    return pd.DataFrame(rows)


def run_evaluation(pivots: dict[str, pd.DataFrame], n_per_dataset: int = 40, seed: int = 42) -> pd.DataFrame:
    holdouts = sample_holdouts(pivots, n_per_dataset=n_per_dataset, seed=seed)
    cross_section_cache: dict[tuple[str, int], pd.DataFrame | None] = {}

    results = []
    # .to_dict("records") statt .itertuples(): vermeidet die unpräzise
    # Scalar-Typisierung der itertuples-Attribute bei gemischten Spaltentypen.
    for record in holdouts.to_dict("records"):
        slug = str(record["dataset"])
        region = str(record["region"])
        year = int(record["year"])
        actual = float(record["actual"])

        est1 = method_1_linear(pivots, slug, region, year)
        est2 = method_2_naive_carry_forward(pivots, slug, region, year)
        est3 = method_3_peer_trend(pivots, slug, region, year)
        est4, r2 = method_4_regression(cross_section_cache, pivots, slug, region, year)

        results.append(
            {
                "dataset": slug,
                "region": region,
                "year": year,
                "actual": actual,
                "est_1_linear": est1,
                "est_2_naive": est2,
                "est_3_peer": est3,
                "est_4_regression": est4,
                "r2_4": r2,
            }
        )

    df = pd.DataFrame(results)
    for m in ["1_linear", "2_naive", "3_peer", "4_regression"]:
        df[f"err_{m}"] = df[f"est_{m}"] - df["actual"]
        df[f"abs_err_{m}"] = df[f"err_{m}"].abs()
        with np.errstate(divide="ignore", invalid="ignore"):
            df[f"pct_err_{m}"] = np.where(
                df["actual"] != 0, df[f"abs_err_{m}"] / df["actual"].abs() * 100, np.nan
            )
    return df


def summarize_evaluation(df: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    methods = ["1_linear", "2_naive", "3_peer", "4_regression"]
    labels = {
        "1_linear": "1. Lineare Interpolation",
        "2_naive": "2. Naive Fortschreibung",
        "3_peer": "3. Peer-basierte Fortschreibung",
        "4_regression": "4. Regressionsschätzung",
    }

    print("#" * 78)
    print(f"# Systematische Evaluation (Leave-one-out, n={len(df)} Stichproben"
          f" über {df['dataset'].nunique()} Datensätze)")
    print("#" * 78)
    print()

    print("--- Gesamt-Gütemaße je Methode ---")
    overall_rows = []
    for m in methods:
        col = df[f"abs_err_{m}"]
        signed = df[f"err_{m}"]
        n_available = col.notna().sum()
        mae = col.mean()
        rmse = np.sqrt((signed**2).mean())
        mape = df[f"pct_err_{m}"].mean()
        bias = signed.mean()
        overall_rows.append(
            {
                "Methode": labels[m],
                "n verfügbar": n_available,
                "MAE": round(mae, 3),
                "Median AE": round(col.median(), 3),
                "RMSE": round(rmse, 3),
                "Std (signed err)": round(signed.std(), 3),
                "MAPE %": round(mape, 2),
                "Median APE %": round(df[f"pct_err_{m}"].median(), 2),
                "Bias (mean signed err)": round(bias, 3),
                "P90 AE": round(col.quantile(0.90), 3),
                "P95 AE": round(col.quantile(0.95), 3),
                "Max AE": round(col.max(), 3),
            }
        )
    overall = pd.DataFrame(overall_rows).set_index("Methode")
    print(overall.to_string())
    print(
        "(AE = Absolute Error, APE = Absolute Percentage Error; P90/P95 zeigen"
        " die Fehlergröße in den schlechtesten 10%/5% der Schätzungen)"
    )
    print()

    print("--- Trefferquote (welche Methode hat je Stichprobe den kleinsten Fehler?) ---")
    err_cols = [f"abs_err_{m}" for m in methods]
    valid = df.dropna(subset=err_cols)
    # np.argmin statt DataFrame.idxmin(axis=1): idxmin ist in den verwendeten
    # pandas-Stubs nur für axis=0/"index" zuverlässig typisiert - der manuelle
    # Weg über numpy ist hier eindeutig typisiert und genauso schnell.
    method_names = [labels[m] for m in methods]
    err_matrix = valid[err_cols].to_numpy(dtype=float)
    winner_idx = np.argmin(err_matrix, axis=1)
    winners = pd.Series([method_names[i] for i in winner_idx], index=valid.index)
    win_counts = winners.value_counts()
    win_pct = (win_counts / len(valid) * 100).round(1)
    win_table = pd.DataFrame({"Anzahl Siege": win_counts, "Anteil %": win_pct})
    print(win_table.to_string())
    print(f"(nur Stichproben gewertet, bei denen alle {len(methods)} Methoden verfügbar waren: n={len(valid)})")
    print()

    print("--- MAE je Datensatz und Methode ---")
    per_dataset_raw = df.groupby("dataset")[err_cols].mean().round(3)
    # DataFrame neu mit den Klartext-Spaltennamen aufbauen statt .rename():
    # vermeidet eine pandas-Stub-Inkompatibilität bei .rename(columns=dict).
    per_dataset = pd.DataFrame(
        per_dataset_raw.to_numpy(), index=per_dataset_raw.index, columns=method_names
    )
    print(per_dataset.to_string())
    print()

    return overall.reset_index(), win_table.reset_index(names="Methode"), per_dataset.reset_index()


RESULTS_DIR = os.path.join(os.path.dirname(__file__), "results")


def save_results(
    demo_df: pd.DataFrame,
    eval_df: pd.DataFrame,
    overall_summary: pd.DataFrame,
    win_table: pd.DataFrame,
    per_dataset_summary: pd.DataFrame,
) -> None:
    os.makedirs(RESULTS_DIR, exist_ok=True)
    demo_df.to_csv(os.path.join(RESULTS_DIR, "demo_known_gaps.csv"), index=False)
    eval_df.to_csv(os.path.join(RESULTS_DIR, "evaluation_holdout_samples.csv"), index=False)
    overall_summary.to_csv(os.path.join(RESULTS_DIR, "evaluation_summary_overall.csv"), index=False)
    win_table.to_csv(os.path.join(RESULTS_DIR, "evaluation_summary_win_rate.csv"), index=False)
    per_dataset_summary.to_csv(os.path.join(RESULTS_DIR, "evaluation_summary_per_dataset.csv"), index=False)
    print(f"Ergebnisse gespeichert unter: {RESULTS_DIR}")
    for fname in [
        "demo_known_gaps.csv",
        "evaluation_holdout_samples.csv",
        "evaluation_summary_overall.csv",
        "evaluation_summary_win_rate.csv",
        "evaluation_summary_per_dataset.csv",
    ]:
        print(f"  - {fname}")


def main() -> None:
    conn = psycopg2.connect(
        host=PGHOST, port=PGPORT, dbname=PGDATABASE, user=PGUSER, password=PGPASSWORD
    )
    try:
        pivots = load_pivots(conn)

        demo_df = demo_known_gaps(conn, pivots)

        eval_df = run_evaluation(pivots, n_per_dataset=40, seed=42)
        overall_summary, win_table, per_dataset_summary = summarize_evaluation(eval_df)

        save_results(demo_df, eval_df, overall_summary, win_table, per_dataset_summary)
    finally:
        conn.close()


if __name__ == "__main__":
    main()

