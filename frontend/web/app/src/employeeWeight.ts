// Wording of the weighting count of 'weighted_avg_employees' aggregations.
// Custom regions deliver the count's dataset slug per row (employee_slug);
// the Agentur endpoint does not, hence AGENTUR_EMPLOYEE_SLUG (mirror of the
// 'employee_slug' entries in mart_agentur_dataset_aggregate.sql).
const EMPLOYEE_BASIS: Record<string, string> = {
  sv_beschaeftigte: "SV-Beschäftigten",
};
const DEFAULT_BASIS = "Erwerbstätigen"; // erwerbstaetige_pro_1000_einwohner

export const AGENTUR_EMPLOYEE_SLUG: Record<string, string> = {
  auszubildende: "sv_beschaeftigte",
};

export function employeeBasis(employeeSlug?: string | null): string {
  return (employeeSlug && EMPLOYEE_BASIS[employeeSlug]) || DEFAULT_BASIS;
}
