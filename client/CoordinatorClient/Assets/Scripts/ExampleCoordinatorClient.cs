using System.Collections;
using UnityEngine;

namespace Coordinator
{
    public class ExampleCoordinatorClient : MonoBehaviour
    {
        public string Address = "127.0.0.1";
        public int PollRate = 60;
        public ulong UserID = 0;
        public string AuthToken = "";
        public string Region = "na";

        private ICoordinatorClientAPI _api;
        private bool _connected = false;
        private Coroutine _pollRoutine;

        private IEnumerator Start()
        {
            _api = new ExampleCoordinatorClientAPI(Address, PollRate);

            IEnumerator startRoutine;
            if(_api.StartSearch(UserID, AuthToken, Region, OnConnectFailed, out startRoutine))
            {
                _connected = true;
                yield return startRoutine;
            }
            else
            {
                yield break;
            }
            
            if(_connected)
            {
                _pollRoutine = StartCoroutine(_api._Poll(OnMatchFound, OnSearchFailed));
            }

        }

        private void OnConnectFailed()
        {
            _connected = false;

            Debug.Log("Failed to connect to the coordinator.");
        }

        private void OnMatchFound(MatchFoundInfo info)
        {
            Debug.Log(string.Format("Found Match: Server = ({0}, {1}), JoinToken = ({2})", info.ServerAddress, info.ServerPort, info.JoinGameToken));
        }

        private void OnSearchFailed()
        {
            _connected = false;

            StopCoroutine(_pollRoutine);

            Debug.Log("Failed to find a match.");

            IEnumerator cancelRoutine;
            _api.CancelSearch(OnCancelSearch, out cancelRoutine);

            StartCoroutine(cancelRoutine);
        }

        private void OnCancelSearch()
        {
            StopCoroutine(_pollRoutine);

            Debug.Log("Search cancelled.");
        }
    }
}
