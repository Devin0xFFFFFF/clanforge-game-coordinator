using System;
using System.Collections;

namespace Coordinator
{
    public interface ICoordinatorClientAPI
    {
        bool StartSearch(ulong userID, string authToken, string region, Action onConnectFailed, out IEnumerator routine);
        bool CancelSearch(Action onCancel, out IEnumerator routine);
        IEnumerator _Poll(Action<MatchFoundInfo> onMatchFound, Action onSearchFailed);
    }
}
