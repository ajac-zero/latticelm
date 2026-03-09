<template>
  <div id="app">
    <router-view />
  </div>
</template>

<script setup lang="ts">
</script>

<style>
:root {
  --background: #0d0d0f;
  --foreground: #e4e4e7;
  --card: #16161a;
  --card-foreground: #e4e4e7;
  --primary: #8b85ff;
  --primary-foreground: #fafafa;
  --secondary: #1e1e24;
  --secondary-foreground: #e4e4e7;
  --muted: #28282e;
  --muted-foreground: #a1a1aa;
  --accent: #8b85ff;
  --border: rgba(255, 255, 255, 0.08);
  --input: rgba(255, 255, 255, 0.08);
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
  background-color: var(--background);
  color: var(--foreground);
}

#app {
  min-height: 100vh;
}
</style>
