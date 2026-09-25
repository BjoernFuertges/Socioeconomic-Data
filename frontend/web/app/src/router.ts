import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";

// regionId ist über alle Gebietstypen hinweg eindeutig (NUTS-Codes, LAU
// gisco_ids und Agentur ba_schl haben disjunkte Formate), der Typ selbst
// steht explizit in ?type= statt als Pfadsegment - RegionBrowser.vue zeigt
// alle drei Gebietstypen in einer kombinierten, per ?types= filterbaren
// Liste, keine Typ-Tabs mehr.
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
