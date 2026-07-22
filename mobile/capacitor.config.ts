import type { CapacitorConfig } from "@capacitor/cli";

const config: CapacitorConfig = {
  appId: "ru.mychat.app",
  appName: "my-chat",
  webDir: "dist",
  server: {
    // В dev-режиме проксируем на локальный бэкенд.
    // При сборке для симулятора закомментировать и использовать hardcoded URLs.
    // url: 'http://localhost:5173',
    // cleartext: true,
  },
  ios: {
    contentInset: "automatic",
  },
};

export default config;
