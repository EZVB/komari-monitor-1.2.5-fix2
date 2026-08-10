// routes.js
import { lazy } from "react";
import type { RouteObject } from "react-router-dom";
import React from "react";

const Index = lazy(() => import("./pages/Index"));
const AdminLayout = lazy(() => import("./pages/admin/_layout"));
const Admin = lazy(() => import("./pages/admin"));
const NotFound = lazy(() => import("./pages/404"));

export const routes: RouteObject[] = [
  {
    path: "/",
    element: React.createElement(lazy(() => import("./pages/_layout"))),
    children: [
      { index: true, element: React.createElement(Index) },
      {
        path: "instance/:uuid",
        element: React.createElement(lazy(() => import("./pages/instance"))),
      },
    ],
  },
  {
    path: "/install",
    element: React.createElement(lazy(() => import("./pages/install"))),
  },
  {
    path: "/database-recovery",
    element: React.createElement(
      lazy(() => import("./pages/database_recovery")),
    ),
  },
  {
    path: "/admin",
    element: React.createElement(AdminLayout),
    children: [
      { index: true, element: React.createElement(Admin) },
      {
        path: "update/1.2.7",
        element: React.createElement(
          lazy(() => import("./pages/admin/update_1_2_7"))
        ),
      },
      {
        path: "theme_managed",
        element: React.createElement(
          lazy(() => import("./pages/admin/theme_managed.tsx"))
        ),
      },
      {
        path: "theme_raw",
        element: React.createElement(
          lazy(() => import("./pages/admin/theme_raw.tsx"))
        ),
      },
      {
        path: "sessions",
        element: React.createElement(
          lazy(() => import("./pages/admin/sessions"))
        ),
      },
      {
        path: "account",
        element: React.createElement(
          lazy(() => import("./pages/admin/account"))
        ),
      },
      {
        path: "settings",
        element: React.createElement(
          lazy(() => import("./pages/admin/settings/_layout"))
        ),
        children: [
          {
            path: "site",
            element: React.createElement(
              lazy(() => import("./pages/admin/settings/site"))
            ),
          },
          {
            path: "theme",
            element: React.createElement(
              lazy(() => import("./pages/admin/settings/theme"))
            ),
          },
          {
            path: "custom",
            element: React.createElement(
              lazy(() => import("./pages/admin/settings/custom"))
            ),
          },
          {
            path: "sign-on",
            element: React.createElement(
              lazy(() => import("./pages/admin/settings/sign-on"))
            ),
          },
          {
            path: "notification",
            element: React.createElement(
              lazy(() => import("./pages/admin/settings/notification"))
            ),
          },
          {
            path: "general",
            element: React.createElement(
              lazy(() => import("./pages/admin/settings/general"))
            ),
          },
          {
            path: "metrics",
            element: React.createElement(
              lazy(() => import("./pages/admin/settings/metrics"))
            ),
          },
        ],
      },
      {
        path: "notification",
        children: [
          {
            path: "offline",
            element: React.createElement(
              lazy(() => import("./pages/admin/notification/offline"))
            ),
          },
          {
            path: "load",
            element: React.createElement(
              lazy(() => import("./pages/admin/notification/load"))
            ),
          },
          {
            path: "general",
            element: React.createElement(
              lazy(() => import("./pages/admin/notification/general"))
            ),
          },
          {
            path: "traffic-report",
            element: React.createElement(
              lazy(() => import("./pages/admin/notification/traffic_report"))
            ),
          },
        ],
      },
      {
        path: "ping",
        element: React.createElement(
          lazy(() => import("./pages/admin/pingTask"))
        ),
      },
      {
        path: "logs",
        element: React.createElement(lazy(() => import("./pages/admin/log"))),
      },
    ],
  },
  {
    path: "/manage/*",
    element: React.createElement(lazy(() => import("./pages/manage"))),
  },
  // Catch-all 404 route
  { path: "*", element: React.createElement(NotFound) },
];
