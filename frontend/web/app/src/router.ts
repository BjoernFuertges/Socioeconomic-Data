import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";

// regionId is unique across all area types (NUTS codes, LAU gisco_ids and Agentur
// ba_schl have disjoint formats); the type itself is explicit in ?type= rather than a
// path segment. RegionBrowser.vue shows all three area types in one combined list
// filterable via ?types=, no type tabs anymore.
const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/regionen" },
  {
    path: "/regionen/:regionId?",
    name: "regionen",
    component: () => import("./components/RegionBrowser.vue"),
  },
  {
    path: "/karte",
    name: "karte",
    component: () => import("./components/MapView.vue"),
  },
  {
    path: "/eigene-region",
    name: "eigene-region",
    component: () => import("./components/CustomRegionView.vue"),
  },
  {
    path: "/katalog/:slug?",
    name: "katalog",
    component: () => import("./components/DatasetCatalog.vue"),
  },
  { path: "/:pathMatch(.*)*", redirect: "/regionen" },
];

export const router = createRouter({
  history: createWebHistory(),
  routes,
});
