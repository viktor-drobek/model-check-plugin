/* probe: channels as message fields
 * claim: the reference's §3 says this is INSIDE the subset
 */
chan reply = [1] of { byte };
chan req = [1] of { chan };
active proctype S() { req!reply }
