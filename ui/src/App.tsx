import React, { useEffect } from 'react';
import './App.scss';
import './configs/antd/customized.css';
import 'react-toastify/dist/ReactToastify.css';
import ReleaseApp from "./pages/ReleaseApp";

import { ToastContainer } from 'react-toastify';

// import AppRoute from './routes/routes';

function App() {
  return (
    <>
      <ReleaseApp />
      {/* <ToastContainer
        position="top-center"
        autoClose={2500}
        hideProgressBar={false}
        newestOnTop={false}
        closeOnClick
        rtl={false}
        pauseOnFocusLoss
        draggable
      /> 
       <AppRoute /> */}
    </>
  );
}

export default App;
