import type { Metadata } from "next";
import { DashboardRootLayout, createDashboardMetadata } from "@sandbox0/dashboard-app";
import "@sandbox0/ui/globals.css";

export const metadata: Metadata = createDashboardMetadata();

export default DashboardRootLayout;
