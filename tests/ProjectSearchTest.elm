module ProjectSearchTest exposing (tests)

import Domain exposing (Project)
import Expect
import ProjectSearch
import Test exposing (Test, describe, test)


tests : Test
tests =
    let
        scotland : Project
        scotland =
            { id = "national-library-of-scotland"
            , name = "National Library of Scotland"
            , homepage = "https://www.nls.uk/"
            , manifestUrl = Just "https://api.nls.uk/iiif/manifest/123"
            , imageInfoUrl = Nothing
            , notes = Just "Uses a browser-compatible request workaround."
            }
    in
    describe "project search"
        [ test "matches names case-insensitively" <|
            \_ -> Expect.equal True (ProjectSearch.matches "Scotland" scotland)
        , test "requires every keyword but not their order" <|
            \_ -> Expect.equal True (ProjectSearch.matches "scotland national" scotland)
        , test "matches identifiers and endpoint hostnames" <|
            \_ -> Expect.equal True (ProjectSearch.matches "api.nls" scotland)
        , test "matches project notes" <|
            \_ -> Expect.equal True (ProjectSearch.matches "browser-compatible" scotland)
        , test "rejects a project when one keyword is absent" <|
            \_ -> Expect.equal False (ProjectSearch.matches "Scotland Oxford" scotland)
        , test "an empty search includes every project" <|
            \_ -> Expect.equal True (ProjectSearch.matches "   " scotland)
        ]
