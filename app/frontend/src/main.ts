import './lib/legacystorage' // first: renames pre-panal localStorage keys before anything reads them
import { mount } from 'svelte'
import './style.css'
import App from './App.svelte'
import { applyStoredAppearance } from './lib/prefs.svelte'

applyStoredAppearance()

const app = mount(App, { target: document.getElementById('app')! })

export default app
