import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";

// Pfade orientieren sich an den Backend-Routen (siehe backend/internal/routes/routes.go),
// damit z.B. /regionen/nuts/DE300 und die API-Route /nuts/DE300 leicht vergleichbar
// bleiben (FAIR-Findability: teilbare/bookmarkbare Deep-Links pro Ansicht).
const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/regionen" },
  {
    path: "/regionen/:regionType(nuts|lau|agentur)?/:regionId?",
    name: "regionen",
    component: () => import("./components/RegionBrowser.vue"),
  },
  {
    path: "/karte",
    name: "karte",
    component: () => import("./components/MapView.vue"),
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
