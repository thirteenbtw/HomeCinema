import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { AppProvider, useApp } from './state';
import { Shell } from './components/Shell';
import Home from './pages/Home';
import Library from './pages/Library';
import Search from './pages/Search';
import { MoviePage, ShowPage } from './pages/Movie';
import Catalog from './pages/Catalog';
import CatalogTitlePage from './pages/CatalogTitle';
import Player from './pages/Player';
import Torrents from './pages/Torrents';
import Devices from './pages/Devices';
import Settings from './pages/Settings';
import ProfilesPage from './pages/Profiles';
import { Login, Setup } from './pages/Setup';
import { tr } from './i18n';

function Gate() {
  const { state } = useApp();
  if (!state) return <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><span className="spin" /></div>;
  if (state.authRequired) return <Login />;
  if (!state.setupDone && state.local) return <Setup />;
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<Home />} />
        <Route path="library/:kind" element={<Library />} />
        <Route path="search" element={<Search />} />
        <Route path="catalog" element={<Catalog />} />
        <Route path="catalog/:kind/:id" element={<CatalogTitlePage />} />
        <Route path="movie/:id" element={<MoviePage />} />
        <Route path="show/:id" element={<ShowPage />} />
        <Route path="play/torrent/:hash" element={<Player />} />
        <Route path="play/:fileId" element={<Player />} />
        <Route path="torrents" element={<Torrents />} />
        <Route path="devices" element={<Devices />} />
        <Route path="settings" element={<Settings />} />
        <Route path="profiles" element={<ProfilesPage />} />
        <Route path="*" element={<main className="main pad"><h1 className="h1">{tr('Страница не найдена', 'Page not found')}</h1></main>} />
      </Route>
    </Routes>
  );
}

export default function App() {
  return (
    <AppProvider>
      <BrowserRouter>
        <Gate />
      </BrowserRouter>
    </AppProvider>
  );
}
